//go:build wasip1

package challenge

import (
	"context"
	"log/slog"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

func init() {
	slog.SetDefault(slog.New(&logHandler{}))
}

// logHandler just implements an slog handler that sends logs to the host.
// attributes, groups, etc. are not supported yet (will be added in the future, maybe).
type logHandler struct{}

func (h *logHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *logHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *logHandler) WithGroup(_ string) slog.Handler { return h }

func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	_, err := api.Log(api.LogInput{
		Severity: r.Level,
		Message:  r.Message,
	})
	return err
}
