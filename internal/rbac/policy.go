package rbac

import (
	"time"

	"github.com/gobwas/glob"
)

// policy provides a local cached representation of the users access rights.
// Access is modeled as list of glob patterns that must match the gRPC procedure name.
type policy struct {
	expires     time.Time
	permissions map[string][]glob.Glob
}

func (p *policy) check(procedure string) []string {
	// add 30 second threshold to ensure that the request is not immediately cancelled
	// instead the check is rejected to refetch from database.
	if p.expires.Before(time.Now().Add(time.Second * 30)) {
		return nil
	}
	scopes := []string{}
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
