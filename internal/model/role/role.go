package role

import (
	"codeberg.org/megakuul/cloudjam/internal/model"
)

const Key model.Partition = "ROLE#"

const SortData model.Sort = "DATA"

type Scope string

const (
	// builtin pseudo scope (not attached to any resource but allows to operate on your owned resources)
	ScopeSelf Scope = "self"
	// builtin scope that allows privilege escalation (hardcoded) in `ConfigureRole` for root access.
	ScopeAdmin Scope = "admin"
)

type Data struct {
	PK          model.PartitionValue `docstore:"pk"`
	SK          model.SortValue      `docstore:"sk"`
	Name        string               `docstore:"name"`
	Description string               `docstore:"description"`
	Builtin     bool                 `docstore:"builtin"`
	Permissions map[Scope][]string   `docstore:"permissions"`

	Scope            Scope `docstore:"scope"`
	DocstoreRevision any
}
