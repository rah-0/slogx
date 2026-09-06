package slogx

import (
	"context"
	"log/slog"
)

type contextAttrsKey struct{}

// WithAttrs returns a context carrying attrs for loggers created by [New] or
// [NewDefault], including the default logger installed by [SetDefault]. Use a
// context-aware logging method, such as InfoContext or [ErrorContext], to include
// them in a record. Other context values are not collected automatically.
//
// Attributes accumulate from parent to child without changing the parent.
// The attribute slice is copied, but values held by attributes are not deep
// copied and must not be mutated concurrently with logging.
// Context attributes follow logger-bound attributes and precede call-site
// attributes, retaining duplicate keys and the logger's current group.
// A nil ctx uses [context.Background]. If attrs is empty, WithAttrs returns
// that context unchanged.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(attrs) == 0 {
		return ctx
	}
	parent, _ := ctx.Value(contextAttrsKey{}).([]slog.Attr)
	combined := make([]slog.Attr, len(parent)+len(attrs))
	copy(combined, parent)
	copy(combined[len(parent):], attrs)
	return context.WithValue(ctx, contextAttrsKey{}, combined)
}

type contextHandler struct {
	handler slog.Handler
}

func (handler contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.handler.Enabled(ctx, level)
}

func (handler contextHandler) Handle(ctx context.Context, record slog.Record) error {
	var attrs []slog.Attr
	if ctx != nil {
		attrs, _ = ctx.Value(contextAttrsKey{}).([]slog.Attr)
	}
	trace := SpanContextFromContext(ctx)
	if len(attrs) == 0 && !trace.IsValid() {
		return handler.handler.Handle(ctx, record)
	}
	// Build a fresh record to prepend metadata without changing the caller's
	// record. The underlying handler applies its active group to every attribute.
	enriched := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	if trace.IsValid() {
		enriched.AddAttrs(slog.String("trace_id", trace.TraceID), slog.String("span_id", trace.SpanID))
	}
	enriched.AddAttrs(attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		enriched.AddAttrs(attr)
		return true
	})
	return handler.handler.Handle(ctx, enriched)
}

func (handler contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handler.handler = handler.handler.WithAttrs(attrs)
	return handler
}

func (handler contextHandler) WithGroup(name string) slog.Handler {
	handler.handler = handler.handler.WithGroup(name)
	return handler
}
