// Package logging provides structured JSON logging with request-scoped
// attributes carried in the context and redaction of sensitive keys.
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// redactedKeys never reach log output, whatever level or source.
var redactedKeys = map[string]struct{}{
	"password": {}, "token": {}, "secret": {}, "cookie": {}, "authorization": {},
	"session": {}, "code": {}, "meeting_code": {}, "display_name": {}, "text": {},
}

type ctxKey struct{}

// New returns a JSON logger that enriches records with context attributes.
func New(w io.Writer, level slog.Level) *slog.Logger {
	base := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redact,
	})
	return slog.New(contextHandler{Handler: base})
}

// With returns a context whose log records carry the given attributes.
func With(ctx context.Context, attrs ...slog.Attr) context.Context {
	existing, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	merged := make([]slog.Attr, 0, len(existing)+len(attrs))
	merged = append(append(merged, existing...), attrs...)
	return context.WithValue(ctx, ctxKey{}, merged)
}

func redact(_ []string, a slog.Attr) slog.Attr {
	if _, ok := redactedKeys[strings.ToLower(a.Key)]; ok {
		return slog.String(a.Key, "[redacted]")
	}
	return a
}

// contextHandler adds context attributes and the trace ID to each record.
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(ctxKey{}).([]slog.Attr); ok {
		r.AddAttrs(attrs...)
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}
