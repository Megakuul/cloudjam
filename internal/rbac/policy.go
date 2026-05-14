package rbac

import (
	"time"

	"codeberg.org/megakuul/cloudjam/internal/model/role"
	"github.com/gobwas/glob"
)

// policy provides a local cached representation of the users access rights.
// Access is modeled as list of glob patterns that must match the gRPC procedure name.
type policy struct {
	expires     time.Time
	permissions map[role.Scope][]glob.Glob
}

func (p *policy) check(procedure string) []role.Scope {
	// add 30 second threshold to ensure that the request is not immediately cancelled
	// instead the check is rejected to refetch from database.
	if p.expires.Before(time.Now().Add(time.Second * 30)) {
		return nil
	}
	scopes := []role.Scope{}
	for scope, exprs := range p.permissions {
		for _, expr := range exprs {
			if expr.Match(procedure) {
				scopes = append(scopes, scope)
				break
			}
		}
	}
	return scopes
}
