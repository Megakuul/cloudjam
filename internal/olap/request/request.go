// package request provides an olap model for system request metrics.
package request

import (
	"context"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array/arreflect"
	"github.com/polarsignals/frostdb/query"
	"github.com/polarsignals/frostdb/query/logicalplan"
)

// Request table is used to track requests.
type Request struct {
	Timestamp uint64
	Endpoint  string
	UserAgent string
	Stream    bool
	Source    string
}

type Controller struct {
	table  string
	engine *query.LocalEngine
}

func New(table string, engine *query.LocalEngine) *Controller {
	return &Controller{
		table:  table,
		engine: engine,
	}
}

// Range returns the specified time range of logs.
func (l *Controller) Range(ctx context.Context, from, until time.Time, limit int) ([]Request, error) {
	requests := []Request{}
	err := l.engine.ScanTable(l.table).Project(logicalplan.All()).
		Filter(logicalplan.And(
			logicalplan.Col("timestamp").GtEq(logicalplan.Literal(from.Unix())),
			logicalplan.Col("timestamp").LtEq(logicalplan.Literal(until.Unix())),
		)).Limit(logicalplan.Literal(limit)).Execute(ctx, func(ctx context.Context, r arrow.RecordBatch) error {
		requestBatch, err := arreflect.RecordToSlice[Request](r)
		if err != nil {
			return err
		}
		requests = append(requests, requestBatch...)
		return nil
	})
	return requests, err
}
