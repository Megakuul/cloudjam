// package request provides an olap model for system request metrics.
package request

import (
	"context"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/system"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/polarsignals/frostdb/query"
	"github.com/polarsignals/frostdb/query/logicalplan"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Request table is used to track requests.
type Request struct {
	Timestamp uint64
	Endpoint  string
	Latency   int64
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

// Aggregate scans all requests from - until and returns $points number of windows with aggregated data.
func (l *Controller) Aggregate(ctx context.Context, from, until time.Time, points int) ([]*system.RequestWindow, error) {
	scale := points / int(until.Unix()-from.Unix())
	windows := []*system.RequestWindow{}
	err := l.engine.ScanTable(l.table).Project(logicalplan.All()).
		Filter(logicalplan.And(
			logicalplan.Col("timestamp").GtEq(logicalplan.Literal(from.Unix())),
			logicalplan.Col("timestamp").LtEq(logicalplan.Literal(until.Unix())),
		)).
		Aggregate([]*logicalplan.AggregationFunction{
			logicalplan.Sum(logicalplan.Col("latency")),
			logicalplan.Count(logicalplan.Col("latency")),
		},
			[]logicalplan.Expr{
				logicalplan.Mul(
					logicalplan.Sub(logicalplan.Col("timestamp"), logicalplan.Literal(from.Unix())),
					logicalplan.Literal(scale),
				).Alias("timestamp"),
			},
		).
		Execute(ctx, func(ctx context.Context, r arrow.RecordBatch) error {
			defer r.Release()
			for row := range int(r.NumRows()) {
				window := &system.RequestWindow{}
				for col := range int(r.NumCols()) {
					switch r.ColumnName(col) {
					case "timestamp":
						ts, ok := r.Column(col).(*array.Uint64)
						if ok {
							window.Start = timestamppb.New(time.Unix(int64(ts.Value(row)), 0))
						}
					case "sum(latency)":
						latency, ok := r.Column(col).(*array.Int64)
						if ok {
							window.Latency = latency.Value(row)
						}
					case "count(latency)":
						count, ok := r.Column(col).(*array.Int64)
						if ok {
							window.Count = count.Value(row)
						}
					}
				}
				windows = append(windows, window)
			}
			return nil
		})
	return windows, err
}
