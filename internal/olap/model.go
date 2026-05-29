package olap

import (
	"context"
	"encoding/json"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/olap/frost"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/polarsignals/frostdb/query/logicalplan"
)

// Log table is used for self ingested logs of the application.
type Log struct {
	Timestamp uint64
	Level     int
	Message   string
	Labels    map[string]string `frostdb:",asc"`
}

func RetrieveLogs(ctx context.Context, from, until time.Time, limit int) ([]Log, error) {
	table, err := frost.NewTable[Log](nil, "log")
	if err != nil {
		panic(err)
	}
	logs := []Log{}
	table.Query().Project(logicalplan.All()).
		Filter(logicalplan.And(
			logicalplan.Col("timestamp").GtEq(logicalplan.Literal(from.Unix())),
			logicalplan.Col("timestamp").LtEq(logicalplan.Literal(until.Unix())),
		)).Limit(logicalplan.Literal(limit)).Execute(ctx, func(ctx context.Context, r arrow.Record) error {
		raw, err := r.MarshalJSON()
		if err != nil {
			return err
		}
		log := Log{}
		err = json.Unmarshal(raw, &log)
		if err != nil {
			return err
		}
		logs = append(logs, log)
		return nil
	})
	return logs, nil
}

type Request struct {
	Timestamp time.Time
	Service   string
	Source    string
}
