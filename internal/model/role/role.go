package role

import (
	"codeberg.org/megakuul/cloudjam/internal/model"
)

const Key model.Partition = "ROLE#"

const SortData model.Sort = "DATA"

type Data struct {
	PK             model.PartitionValue `docstore:"pk"`
	SK             model.SortValue      `docstore:"sk"`
	Name           string               `docstore:"name"`
	Description    string               `docstore:"description"`
	ProcedureExprs []string             `docstore:"procedure_exprs"`
}
