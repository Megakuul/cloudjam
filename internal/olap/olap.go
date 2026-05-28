// olap provides an abstraction over an underlying olap system used for analytic data that should not be represented in dynamitedb.
// this package also defines the table model for cloudjam olap data.
package olap

import "context"

// Inserter abstracts an OLAP inserter for the generic model.
// Abstraction exists to add a batcher system between the insertion engine and the insert call (to avoid serverless parquet churn).
type Inserter[T any] interface {
	Insert(context.Context, T) error
}

// Extractor abstracts an OLAP query engine for the generic model.
type Extractor[T any] interface {
	Query(context.Context) []T
}
