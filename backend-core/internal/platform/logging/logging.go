package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

const (
	levelDebug = "debug"
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
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

func New(w io.Writer, level, service string, otelHandler slog.Handler) (*slog.Logger, error) {
	parsedLevel, err := ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("create logger: %w", err)
	}
	opts := &slog.HandlerOptions{
		Level: parsedLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			for _, group := range groups {
				if isSecretKey(group) {
					return slog.Attr{Key: a.Key, Value: slog.StringValue(Redacted)}
				}
			}
			return slog.Attr{Key: a.Key, Value: RedactValue(a.Key, a.Value.Resolve())}
		},
	}
	var h slog.Handler = slog.NewJSONHandler(w, opts)
	if otelHandler != nil {
		h = fanout{h, redacting{Handler: otelHandler}}
	}
	return slog.New(levelFilter{Handler: traceContext{h}, level: parsedLevel}).With("service", service), nil
}

type traceContext struct{ slog.Handler }

func (h traceContext) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	if err := h.Handler.Handle(ctx, r); err != nil {
		return fmt.Errorf("handle trace log: %w", err)
	}
	return nil
}

func (h traceContext) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceContext{h.Handler.WithAttrs(attrs)}
}
func (h traceContext) WithGroup(name string) slog.Handler {
	return traceContext{h.Handler.WithGroup(name)}
}

type redacting struct {
	slog.Handler
	secretGroup bool
}

func (h redacting) Handle(ctx context.Context, r slog.Record) error {
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

type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
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

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}

func (h redacting) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = h.redactAttr(a)
	}
	return redacting{h.Handler.WithAttrs(clean), h.secretGroup}
}
func (h redacting) WithGroup(name string) slog.Handler {
	return redacting{h.Handler.WithGroup(name), h.secretGroup || isSecretKey(name)}
}

type levelFilter struct {
	slog.Handler
	level slog.Level
}

func (h levelFilter) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level && h.Handler.Enabled(ctx, level)
}
func (h levelFilter) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelFilter{h.Handler.WithAttrs(attrs), h.level}
}
func (h levelFilter) WithGroup(name string) slog.Handler {
	return levelFilter{h.Handler.WithGroup(name), h.level}
}

func (h redacting) redactAttr(a slog.Attr) slog.Attr {
	if h.secretGroup {
		return slog.Attr{Key: a.Key, Value: slog.StringValue(Redacted)}
	}
	return slog.Attr{Key: a.Key, Value: RedactValue(a.Key, a.Value.Resolve())}
}
