package creds

import (
	"time"

	"codeberg.org/megakuul/cloudjam/internal/model"
	"codeberg.org/megakuul/cloudjam/internal/model/role"
)

const Key model.Partition = "CREDS#"

const SortData model.Sort = "DATA"

type Data struct {
	PK               model.PartitionValue `docstore:"pk"`
	SK               model.SortValue      `docstore:"sk"`
	Active           bool                 `docstore:"active"`
	UserId           string               `docstore:"user_id"`
	Password         string               `docstore:"password"`
	Code             string               `docstore:"code"`
	CodeExpiration   time.Time            `docstore:"code_expiration"`
	DocstoreRevision any

	// defines WHO has access to this datablock
	Scopes []role.Scope `docstore:"scopes"`
}
