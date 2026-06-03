// olap provides writers for the analytic data ingestion system.
// The models including read controllers are defined as subpackages.
package olap

import (
	"context"

	"github.com/polarsignals/frostdb"
)

// Inserter abstracts an OLAP inserter for the generic model.
// Abstraction exists to add a batcher system between the frostdb insertion engine and the insert call (to avoid serverless parquet churn).
type Inserter[T any] interface {
	Insert(context.Context, T) error
}

// LocalInserter directly writes inserts to an underlying frostdb instance.
type LocalInserter[T any] struct {
	table *frostdb.GenericTable[T]
}

func NewLocalInserter[T any](table *frostdb.GenericTable[T]) *LocalInserter[T] {
	return &LocalInserter[T]{
		table: table,
	}
}

func (i *LocalInserter[T]) Insert(ctx context.Context, record T) error {
	_, err := i.table.Write(ctx, record)
	if err != nil {
		return err
	}
	return nil
}
