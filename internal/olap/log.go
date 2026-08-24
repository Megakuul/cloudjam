// Package olap provides datastructures for olap models
// (effectively all data that must be iteratable / aggregatable -> data that is hard to process for dynamitedb).
package olap

import (
	"github.com/megakuul/lake"
)

// Log table is used for self ingested logs of the application.
type Log struct {
	lake.Table `name:"log" sort:"timestamp:desc"`

	Timestamp  lake.Int    `parquet:"timestamp"`
	Level      lake.Int    `parquet:"level"`
	Redirected lake.Int    `parquet:"redirected"` // logs are redirected from external platforms
	Challenge  lake.String `parquet:"challenge"`  // log is a challenge if this is not ""
	Message    lake.String `parquet:"message"`
	System     lake.String `parquet:"system"`
	Procedure  lake.String `parquet:"procedure"`
	Trace      lake.String `parquet:"trace"`
}
