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
func (v *Authorizer) Check(ctx context.Context, subject, procedure string) (time.Time, error) {
	v.cacheLock.RLock()
	cachedPolicy, ok := v.cache[subject]
	if ok {
		if cachedPolicy.check(procedure) {
			return cachedPolicy.expires, nil
		}
	}
	v.cacheLock.RUnlock()

	user := &user.Data{PK: user.Key.New(subject), SK: user.SortData.New("")}
	if err := v.coll.Get(ctx, user); err != nil {
		return time.Time{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found: %w", err))
	}
	role := &role.Data{PK: role.Key.New(user.Role), SK: role.SortData.New("")}
	if err := v.coll.Get(ctx, role); err != nil {
		return time.Time{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("role not found: %w", err))
	}
	policy := policy{
		expires: time.Now().Add(v.cacheTimeout),
		exprs:   []glob.Glob{},
	}
	for _, rawExpr := range role.ProcedureExprs {
		expr, err := glob.Compile(rawExpr, '/')
		if err != nil {
			return time.Time{}, connect.NewError(connect.CodeInternal, fmt.Errorf("role policy contains invalid matcher: %v", err))
		}
		policy.exprs = append(policy.exprs, expr)
	}

	v.cacheLock.Lock()
	defer v.cacheLock.Unlock()
	v.cache[subject] = policy

	if policy.check(procedure) {
		return policy.expires, nil
	}
	return time.Time{}, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
}
