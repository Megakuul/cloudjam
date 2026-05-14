// package rbac provides an authorizer to enforce rbac based on policies that glob match rpc function names.
// the validator uses a memory local cache to deflect some load from the database (costs around 1 RRU to fetch the policy).
package rbac

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/model/role"
	"codeberg.org/megakuul/cloudjam/internal/model/user"
	"connectrpc.com/connect"
	"github.com/gobwas/glob"
	"gocloud.dev/docstore"
)

type Authorizer struct {
	coll *docstore.Collection

	cacheLock    sync.RWMutex
	cache        map[string]policy
	cacheTimeout time.Duration
}

func New(coll *docstore.Collection, cacheTimeout time.Duration) *Authorizer {
	return &Authorizer{
		coll:         coll,
		cacheLock:    sync.RWMutex{},
		cache:        map[string]policy{},
		cacheTimeout: cacheTimeout,
	}
}

// Check verifies that the provided subject has access to the procedure (gRPC procedure name).
// The function performs in memory caching for the happy path in a defined timeout (thread safe).
// Returns the cache expiration time of the applied policy; it's advisable to reconnect streams at this time.
func (v *Authorizer) Check(ctx context.Context, subject, procedure string) (time.Time, []role.Scope, error) {
	v.cacheLock.RLock()
	cachedPolicy, ok := v.cache[subject]
	if ok {
		if scopes := cachedPolicy.check(procedure); len(scopes) > 0 {
			v.cacheLock.RUnlock()
			return cachedPolicy.expires, scopes, nil
		}
	}
	v.cacheLock.RUnlock()

	userIter := v.coll.Query().
		Where("pk", "=", user.Key.New(subject)).
		Where("sk", "=", user.SortData.New("")).
		Get(ctx)
	defer userIter.Stop()
	user := &user.Data{}
	if err := userIter.Next(ctx, user); err != nil {
		return time.Time{}, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found: %w", err))
	}
	roleIter := v.coll.Query().
		Where("pk", "=", role.Key.New(user.Role)).
		Where("sk", "=", role.SortData.New("")).
		Get(ctx)
	defer roleIter.Stop()
	roleData := &role.Data{}
	if err := roleIter.Next(ctx, roleData); err != nil {
		return time.Time{}, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role not found: %w", err))
	}
	policy := policy{
		expires:     time.Now().Add(v.cacheTimeout),
		permissions: map[role.Scope][]glob.Glob{},
	}
	for scope, exprs := range roleData.Permissions {
		for _, expr := range exprs {
			compiledExpr, err := glob.Compile(string(expr), '/')
			if err != nil {
				return time.Time{}, nil, connect.NewError(connect.CodeInternal, fmt.Errorf("role policy contains invalid matcher: %v", err))
			}
			policy.permissions[scope] = append(policy.permissions[scope], compiledExpr)
		}
	}

	v.cacheLock.Lock()
	defer v.cacheLock.Unlock()
	v.cache[subject] = policy

	if scopes := policy.check(procedure); len(scopes) > 0 {
		return policy.expires, scopes, nil
	}
	return time.Time{}, nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
}
