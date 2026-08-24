package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/olap"
	"github.com/megakuul/lake"
)

// LogSink implements an slog.Handler that writes directly to the OLAP log table.
// only supports flat string attrs; nested group attributes are discarded.
type LogSink struct {
	min      slog.Level // minimum slog level reported
	attrs    []slog.Attr
	ingestor *lake.Ingestor[olap.Log]
	timeout  time.Duration
}

type LogSinkOptions struct {
	Level   slog.Level
	Timeout time.Duration
}

func NewLogSink(inserter *lake.Ingestor[olap.Log], opts *LogSinkOptions) *LogSink {
	return &LogSink{
		min:      opts.Level,
		ingestor: inserter,
		timeout:  opts.Timeout,
	}
}

func (l *LogSink) Enabled(_ context.Context, level slog.Level) bool {
	return level >= l.min
}

func (l *LogSink) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(l.attrs...)
	log := olap.Log{
		Timestamp: lake.NewInt(r.Time.UnixNano()),
		Level:     lake.NewInt(int64(r.Level)),
		Message:   lake.NewString(r.Message),
		Trace:     lake.NewString(fmt.Sprintf("%s:%d", r.Source().File, r.Source().Line)),
	}
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "system":
			log.System = lake.NewString(a.Value.String())
		case "proc":
			log.Procedure = lake.NewString(a.Value.String())
		case "challenge":
			log.Challenge = lake.NewString(a.Value.String())
		}
		return true
	})
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	return l.ingestor.Insert(ctx, log)
}

func (l *LogSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogSink{
		min:      l.min,
		attrs:    append(l.attrs, attrs...),
		ingestor: l.ingestor,
	}
}

func (l *LogSink) WithGroup(string) slog.Handler {
	// not supported; requires a slog frontend like slog.MultiHandler.
	return l
}
