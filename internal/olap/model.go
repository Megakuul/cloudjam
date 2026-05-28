package olap

import (
	"log/slog"
	"time"
)

// Log table is used for self ingested logs of the application.
type Log struct {
	Timestamp time.Time
	Level     slog.Level
	Message   string
	Labels    map[string]string `frostdb:",asc"`
}

type Request struct {
	Timestamp time.Time
	Service   string
	Source    string
}
