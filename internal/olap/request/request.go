// package request provides an olap model for system request metrics.
package request

import (
	"context"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/system"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/parquet-go/parquet-go"
	"github.com/polarsignals/frostdb/query/logicalplan"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Request table is used to track requests.
type Request struct {
	Timestamp int64  `parquet:"timestamp"`
	Endpoint  string `parquet:"endpoint,zstd"`
	Latency   int64  `parquet:"latency"`
	Stream    bool   `parquet:"stream"`
	Source    string `parquet:"source,zstd"`
}

type Controller struct {
	table parquet.GenericReader[Request]
}

func New(table parquet.GenericReader[Request]) *Controller {
	return &Controller{
		table: table,
	}
}

// Aggregate scans all requests from - until and returns $points number of windows with aggregated data.
// Might return < points request windows because empty windows are just skipped (if they have no data).
func (l *Controller) Aggregate(ctx context.Context, from, until time.Time, points int64) ([]*system.RequestWindow, error) {
	totalRange := int64(until.Unix() - from.Unix())
	windows := []*system.RequestWindow{}
	err := l.engine.ScanTable(l.table).Project(
		logicalplan.Col("timestamp"),
		logicalplan.Col("endpoint"),
		logicalplan.Col("latency"),
		logicalplan.Div(
			logicalplan.Mul(
				logicalplan.Sub(logicalplan.Col("timestamp"), logicalplan.Literal(from.Unix())),
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
			for row := range int(r.NumRows()) {
				window := &system.RequestWindow{}
				for col := range int(r.NumCols()) {
					switch r.ColumnName(col) {
					case "bucket":
						ts, ok := r.Column(col).(*array.Int64)
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
