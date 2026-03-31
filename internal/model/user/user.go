package user

import (
	"time"

	"codeberg.org/megakuul/cloudjam/internal/model"
)

const Key model.Partition = "USER#"

const SortData model.Sort = "DATA"

type Data struct {
	PK          model.PartitionValue `docstore:"pk"`
	SK          model.SortValue      `docstore:"sk"`
	Username    string               `docstore:"username"`
	Description string               `docstore:"description"`
	Email       string               `docstore:"email"`
	CreatedAt   time.Time            `docstore:"created_at"`
	Score       float64              `docstore:"score"`
	Streak      int                  `docstore:"streak"`
	MaxStreak   int                  `docstore:"max_streak"`
	Role        string               `docstore:"role"`
}
