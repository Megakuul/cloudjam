package rbac

import (
	"time"

	"github.com/gobwas/glob"
)

// policy provides a local cached representation of the users access rights.
// Access is modeled as list of glob patterns that must match the gRPC procedure name.
type policy struct {
	expires time.Time
	exprs   []glob.Glob
}

func (p *policy) check(procedure string) bool {
	// add 30 second threshold to ensure that the request is not immediately cancelled
	// instead the check is rejected to refetch from database.
	if p.expires.After(time.Now().Add(time.Second * 30)) {
		return false
	}
	for _, expr := range p.exprs {
		if expr.Match(procedure) {
			return true
		}
	}
	return false
}
