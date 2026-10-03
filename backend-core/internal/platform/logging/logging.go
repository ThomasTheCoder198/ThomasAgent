package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"
)

const (
	levelDebug       = "debug"
	levelInfo        = "info"
	levelWarn        = "warn"
	levelError       = "error"
	fieldTimestamp   = "ts"
	fieldService     = "service"
	fieldEnvironment = "env"
	fieldTraceID     = "trace_id"
	fieldSpanID      = "span_id"
	fieldRequestID   = "request_id"
)

func ParseLevel(level string) (slog.Level, error) {
	switch level {
	case levelDebug:
		return slog.LevelDebug, nil
	case levelInfo:
		return slog.LevelInfo, nil
	case levelWarn:
		return slog.LevelWarn, nil
	case levelError:
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q", level)
	}
}

func NewLogger(w io.Writer, level, service, environment string, otelHandler slog.Handler) (*slog.Logger, error) {
	parsedLevel, err := ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("create logger: %w", err)
	}
	opts := &slog.HandlerOptions{
		Level: parsedLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
				a = slog.String(fieldTimestamp, a.Value.Time().UTC().Format(time.RFC3339Nano))
			}
			for _, group := range groups {
				if isSecretKey(group) {
					return slog.Attr{Key: a.Key, Value: slog.StringValue(RedactedPlaceholder)}
				}
			}
			return slog.Attr{Key: a.Key, Value: RedactValue(a.Key, a.Value.Resolve())}
		},
	}
	var h slog.Handler = slog.NewJSONHandler(w, opts)
	if otelHandler != nil {
		h = multiHandler{h, redactingHandler{Handler: otelHandler}}
	}
	return slog.New(levelFilterHandler{Handler: traceContextHandler{h}, level: parsedLevel}).With(fieldService, service, fieldEnvironment, environment), nil
}

type traceContextHandler struct{ slog.Handler }

func (h traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String(fieldTraceID, sc.TraceID().String()), slog.String(fieldSpanID, sc.SpanID().String()))
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		r.AddAttrs(slog.String(fieldRequestID, requestID))
	}
	if err := h.Handler.Handle(ctx, r); err != nil {
		return fmt.Errorf("handle trace log: %w", err)
	}
	return nil
}

func (h traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceContextHandler{h.Handler.WithAttrs(attrs)}
}
func (h traceContextHandler) WithGroup(name string) slog.Handler {
	return traceContextHandler{h.Handler.WithGroup(name)}
}

type redactingHandler struct {
	slog.Handler
	insideSecretGroup bool
}

func (h redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	clean := slog.NewRecord(r.Time, r.Level, RedactValue("", slog.StringValue(r.Message)).String(), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(h.redactAttr(a))
		return true
	})
	if err := h.Handler.Handle(ctx, clean); err != nil {
		return fmt.Errorf("handle redacted log: %w", err)
	}
	return nil
}

type multiHandler []slog.Handler

func (f multiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (f multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(multiHandler, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f multiHandler) WithGroup(name string) slog.Handler {
	out := make(multiHandler, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = h.redactAttr(a)
	}
	return redactingHandler{h.Handler.WithAttrs(clean), h.insideSecretGroup}
}
func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{h.Handler.WithGroup(name), h.insideSecretGroup || isSecretKey(name)}
}

type levelFilterHandler struct {
	slog.Handler
	level slog.Level
}

func (h levelFilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level && h.Handler.Enabled(ctx, level)
}
func (h levelFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelFilterHandler{h.Handler.WithAttrs(attrs), h.level}
}
func (h levelFilterHandler) WithGroup(name string) slog.Handler {
	return levelFilterHandler{h.Handler.WithGroup(name), h.level}
}

func (h redactingHandler) redactAttr(a slog.Attr) slog.Attr {
	if h.insideSecretGroup {
		return slog.Attr{Key: a.Key, Value: slog.StringValue(RedactedPlaceholder)}
	}
	return slog.Attr{Key: a.Key, Value: RedactValue(a.Key, a.Value.Resolve())}
}
