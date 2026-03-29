package role

import (
	"codeberg.org/megakuul/cloudjam/internal/model"
)

const Key model.Partition = "ROLE#"

const SortData model.Sort = "DATA"

type Data struct {
	PK             model.Partition `docstore:"pk"`
	SK             model.Sort      `docstore:"sk"`
	Name           string          `docstore:"name"`
	Description    string          `docstore:"description"`
	ProcedureExprs []string        `docstore:"procedure_exprs"`
}
