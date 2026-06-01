// package log provides an olap model for system logs.
// The idea of this model is to provide the system with its own logs so it is not dependent on an external logging system.
package log

import (
	"context"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array/arreflect"
	"github.com/polarsignals/frostdb/query"
	"github.com/polarsignals/frostdb/query/logicalplan"
)

// Log table is used for self ingested logs of the application.
type Log struct {
	Timestamp uint64
	Level     int64
	Message   string
	Labels    map[string]string `frostdb:",asc"`
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
func (l *Controller) Range(ctx context.Context, from, until time.Time, limit int) ([]Log, error) {
	logs := []Log{}
	err := l.engine.ScanTable(l.table).Project(logicalplan.All()).
		Filter(logicalplan.And(
			logicalplan.Col("timestamp").GtEq(logicalplan.Literal(from.Unix())),
			logicalplan.Col("timestamp").LtEq(logicalplan.Literal(until.Unix())),
		)).Limit(logicalplan.Literal(limit)).Execute(ctx, func(ctx context.Context, r arrow.RecordBatch) error {
		logBatch, err := arreflect.RecordToSlice[Log](r)
		if err != nil {
			return err
		}
		logs = append(logs, logBatch...)
		return nil
	})
	return logs, err
}
