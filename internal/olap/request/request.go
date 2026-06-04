// package request provides an olap model for system request metrics.
package request

import (
	"context"
	"fmt"
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
	Timestamp uint64 `frostdb:"timestamp,desc"`
	Endpoint  string `frostdb:"endpoint"`
	Latency   int64  `frostdb:"latency"`
	Stream    bool   `frostdb:"stream"`
	Source    string `frostdb:"source"`
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
// Might return < points request windows because empty windows are just skipped (if they have no data).
func (l *Controller) Aggregate(ctx context.Context, from, until time.Time, points uint64) ([]*system.RequestWindow, error) {
	totalRange := uint64(until.Unix() - from.Unix())
	windows := []*system.RequestWindow{}
	err := l.engine.ScanTable(l.table).Project(
		logicalplan.Col("timestamp"),
		logicalplan.Col("endpoint"),
		logicalplan.Col("latency"),
		logicalplan.Div(
			logicalplan.Mul(
				logicalplan.Sub(logicalplan.Col("timestamp"), logicalplan.Literal(uint64(from.Unix()))),
				logicalplan.Literal(points),
			),
			logicalplan.Literal(totalRange),
		).Alias("bucket"),
	).
		Filter(logicalplan.And(
			logicalplan.Col("timestamp").GtEq(logicalplan.Literal(from.Unix())),
			logicalplan.Col("timestamp").LtEq(logicalplan.Literal(until.Unix())),
		)).
		Aggregate([]*logicalplan.AggregationFunction{
			logicalplan.Sum(logicalplan.Col("latency")),
			logicalplan.Count(logicalplan.Col("latency")),
		},
			[]logicalplan.Expr{
				logicalplan.Col("bucket"),
				logicalplan.Col("endpoint"),
			},
		).
		Execute(ctx, func(ctx context.Context, r arrow.RecordBatch) error {
			defer r.Release()
			fmt.Println(r)
			for row := range int(r.NumRows()) {
				window := &system.RequestWindow{}
				for col := range int(r.NumCols()) {
					switch r.ColumnName(col) {
					case "bucket":
						ts, ok := r.Column(col).(*array.Uint64)
						if ok {
							window.Start = timestamppb.New(time.Unix(int64(ts.Value(row))+from.Unix(), 0))
						}
					case "endpoint":
						endpoint, ok := r.Column(col).(*array.Binary)
						if ok {
							window.Endpoint = string(endpoint.Value(row))
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
