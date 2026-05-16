package user

import (
	"time"

	"codeberg.org/megakuul/cloudjam/internal/model"
	"codeberg.org/megakuul/cloudjam/internal/model/role"
)

const Key model.Partition = "USER#"

const SortData model.Sort = "DATA"

type Data struct {
	PK           model.PartitionValue `docstore:"pk"`
	SK           model.SortValue      `docstore:"sk"`
	PubId        string               `docstore:"pub_id"`
	Username     string               `docstore:"username"`
	Description  string               `docstore:"description"`
	Organization string               `docstore:"organization"`
	Email        string               `docstore:"email"`
	CreatedAt    time.Time            `docstore:"created_at"`
	Score        float64              `docstore:"score"`
	MaxScore     float64              `docstore:"max_score"`
	Streak       int                  `docstore:"streak"`
	MaxStreak    int                  `docstore:"max_streak"`
	Privileged   bool                 `docstore:"privileged"`
	Role         string               `docstore:"role"`

	Scope            role.Scope `docstore:"scope"`
	DocstoreRevision any
}
