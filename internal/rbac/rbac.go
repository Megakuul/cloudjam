// package rbac provides an authorizer to enforce rbac based on policies that glob match rpc function names.
// the validator uses a memory local cache to deflect some load from the database (costs around 1 RRU to fetch the policy).
package rbac

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"connectrpc.com/connect"
	"github.com/gobwas/glob"
	"github.com/megakuul/dynamitedb"
)

type Authorizer struct {
	bucket *dynamitedb.Bucket

	cacheLock    sync.RWMutex
	cache        map[string]policy
	cacheTimeout time.Duration
}

func New(bucket *dynamitedb.Bucket, cacheTimeout time.Duration) *Authorizer {
	return &Authorizer{
		bucket:       bucket,
		cacheLock:    sync.RWMutex{},
		cache:        map[string]policy{},
		cacheTimeout: cacheTimeout,
	}
}

// Check verifies that the provided subject has access to the procedure (gRPC procedure name).
// The function performs in memory caching for the happy path in a defined timeout (thread safe).
// Returns the cache expiration time of the applied policy; it's advisable to reconnect streams at this time.
func (v *Authorizer) Check(ctx context.Context, subject, procedure string) (time.Time, []string, error) {
	v.cacheLock.RLock()
	cachedPolicy, ok := v.cache[subject]
	if ok {
		if scopes := cachedPolicy.check(procedure); len(scopes) > 0 {
			v.cacheLock.RUnlock()
			return cachedPolicy.expires, scopes, nil
		}
	}
	v.cacheLock.RUnlock()

	user, err := dynamitedb.Get(ctx, v.bucket, &oltp.User{
		UserID: dynamitedb.Key(subject),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return time.Time{}, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
		}
		return time.Time{}, nil, connect.NewError(connect.CodeInternal, err)
	}
	role, err := dynamitedb.Get(ctx, v.bucket, &oltp.Role{
		RoleID: dynamitedb.Key(user.Role.Value()),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return time.Time{}, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role not found"))
		}
		return time.Time{}, nil, connect.NewError(connect.CodeInternal, err)
	}

	policy := policy{
		expires:     time.Now().Add(v.cacheTimeout),
		permissions: map[string][]glob.Glob{},
	}
	for scope, exprs := range role.Permissions.Value() {
		for _, expr := range strings.Split(exprs, ",") {
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
