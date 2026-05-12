package role

import (
	"codeberg.org/megakuul/cloudjam/internal/model"
)

const Key model.Partition = "ROLE#"

const SortData model.Sort = "DATA"

type Scope string

const (
	ScopeSelf  Scope = "self"  // scopes to data referenced to your id
	ScopeAdmin Scope = "admin" // scopes to everything
)

type Data struct {
	PK          model.PartitionValue `docstore:"pk"`
	SK          model.SortValue      `docstore:"sk"`
	Name        string               `docstore:"name"`
	Description string               `docstore:"description"`
	Builtin     bool                 `docstore:"builtin"`
	// ProcedureExprs defines ACTION access
	ProcedureExprs []string `docstore:"procedure_exprs"`
	// Scope defines DATA access
	Scope Scope `docstore:"scope"`

	// defines WHO has access to this datablock
	Scopes           []Scope `docstore:"scopes"`
	DocstoreRevision any
}
