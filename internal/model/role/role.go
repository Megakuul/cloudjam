package role

import (
	"codeberg.org/megakuul/cloudjam/internal/model"
)

const Key model.Partition = "ROLE#"

const SortData model.Sort = "DATA"

type Scope string

const (
	ScopeAdmin Scope = "admin"
)

type Data struct {
	PK          model.PartitionValue `docstore:"pk"`
	SK          model.SortValue      `docstore:"sk"`
	Name        string               `docstore:"name"`
	Description string               `docstore:"description"`
	Builtin     bool                 `docstore:"builtin"`
	// ProcedureExprs defines ACTION access
	ProcedureExprs []string `docstore:"procedure_exprs"`
	// Scopes define DATA access
	Scopes []Scope `docstore:"scopes"`

	Scope            Scope `docstore:"scope"`
	DocstoreRevision any
}
