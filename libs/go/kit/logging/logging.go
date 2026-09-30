// Package logging produces the JSON log lines described in design section 7:
// one object per line on stdout with time (RFC 3339, milliseconds), level, msg
// and service, plus trace_id, span_id and request_id when the context carries
// them.
//
// Because the identifiers come from the context, use the *Context log methods
// (logger.InfoContext, logger.ErrorContext, ...) on any code path that has one.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// TimeFormat is RFC 3339 with milliseconds, as required by the design.
const TimeFormat = "2006-01-02T15:04:05.000Z07:00"

type requestIDKey struct{}

// WithRequestID returns a context carrying the request id, so that log records
// written with that context include it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request id carried by ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// ParseLevel maps a LOG_LEVEL string onto a slog level. Unknown values are info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New returns a logger that writes JSON lines to stdout.
func New(service, level string) *slog.Logger {
	return NewTo(os.Stdout, service, level)
}

// NewTo is New with an explicit destination; tests use it to capture output.
func NewTo(w io.Writer, service, level string) *slog.Logger {
	base := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       ParseLevel(level),
		ReplaceAttr: replaceAttr,
	})
	return slog.New(&contextHandler{Handler: base}).With(slog.String("service", service))
}

func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		if t, ok := a.Value.Any().(time.Time); ok {
			return slog.String(slog.TimeKey, t.UTC().Format(TimeFormat))
		}
	case slog.LevelKey:
		if l, ok := a.Value.Any().(slog.Level); ok {
			return slog.String(slog.LevelKey, strings.ToLower(l.String()))
		}
	}
	return a
}

// contextHandler decorates every record with the correlation identifiers found
// in the context at the moment the record is written.
type contextHandler struct{ slog.Handler }

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if id := RequestID(ctx); id != "" {
			r.AddAttrs(slog.String("request_id", id))
		}
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			r.AddAttrs(
				slog.String("trace_id", sc.TraceID().String()),
				slog.String("span_id", sc.SpanID().String()),
			)
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}
