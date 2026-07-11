package olap

import (
	"github.com/megakuul/lake"
)

type RequestType int64

const (
	RequestUnary RequestType = iota
	RequestStream
)

// Request table is used to track requests.
type Request struct {
	lake.Table `name:"request" sort:"timestamp:desc"`

	Timestamp lake.Int    `parquet:"timestamp"`
	Endpoint  lake.String `parquet:"endpoint"`
	Latency   lake.Int    `parquet:"latency"`
	Type      lake.Int    `parquet:"type"`
	Source    lake.String `parquet:"source"`
}
