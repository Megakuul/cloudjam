package log

import (
	"context"
	"log/slog"

	"codeberg.org/megakuul/cloudjam/internal/olap"
)

// Sink implements an slog.Handler that writes directly to the OLAP log table.
// only supports flat string attrs; nested group attributes are discarded.
type Sink struct {
	min      slog.Level // minimum slog level reported
	attrs    []slog.Attr
	inserter olap.Inserter[Log]
}

type SinkOptions struct {
	Level slog.Level
}

func NewSink(inserter olap.Inserter[Log], opts *SinkOptions) *Sink {
	return &Sink{
		min:      opts.Level,
		inserter: inserter,
	}
}

func (l *Sink) Enabled(_ context.Context, level slog.Level) bool {
	return level >= l.min
}

func (l *Sink) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(l.attrs...)
	log := Log{
		Timestamp: uint64(r.Time.Unix()),
		Level:     int64(r.Level),
		Message:   r.Message,
		Labels:    map[string]string{},
	}
	r.Attrs(func(a slog.Attr) bool {
		log.Labels[a.Key] = a.Value.String()
		return true
	})
	return l.inserter.Insert(ctx, log)
}

func (l *Sink) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Sink{
		min:      l.min,
		attrs:    append(l.attrs, attrs...),
		inserter: l.inserter,
	}
}

func (l *Sink) WithGroup(string) slog.Handler {
	// not supported; requires a slog frontend like slog.MultiHandler.
	return l
}
