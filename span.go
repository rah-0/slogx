package slogx

import (
	"context"
	"log/slog"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// SpanKind describes the relationship between an operation and its peers.
type SpanKind uint8

const (
	SpanKindInternal SpanKind = iota + 1
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// SpanStatus describes the outcome of an operation, independently of its events.
type SpanStatus uint8

const (
	StatusUnset SpanStatus = iota
	StatusOK
	StatusError
)

type spanEvent struct {
	Time       time.Time
	Name       string
	Attributes []slog.Attr
}

type spanData struct {
	ParentSpanID      string
	Name              string
	Kind              SpanKind
	StartTime         time.Time
	EndTime           time.Time
	Attributes        []slog.Attr
	Events            []spanEvent
	Status            SpanStatus
	StatusDescription string
}

type spanKey struct{}

// Span represents an operation. Use [StartSpan] to create one. Its methods are
// safe for concurrent use; a Span must not be copied after first use.
type Span struct {
	mu      sync.Mutex
	once    sync.Once
	data    spanData
	sc      SpanContext
	ctx     context.Context
	logger  *slog.Logger
	pc      uintptr
	enabled bool
	ended   bool
}

// StartSpan starts an internal operation and returns a derived context carrying
// its identity. A valid parent supplies the trace ID, sampled flag and trace
// state; each operation gets a new span ID. Roots start a new, sampled trace.
// Request cancellation, deadlines and values are preserved in the returned ctx.
// A nil ctx uses [context.Background].
//
// Recording requires a sampled span and an INFO-enabled default logger at start.
// Without recording, IDs still correlate logs, but attributes and errors are not
// evaluated or retained. Span attributes are separate from the log attributes
// attached by [WithAttrs].
//
// The default logger and caller are captured at start. [Span.End] writes through
// that logger, so its handler applies AddSource, ReplaceAttr and groups just as
// for ordinary logs. The source, when enabled, identifies the StartSpan caller.
func StartSpan(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, *Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	parent := SpanContextFromContext(ctx)
	sc := SpanContext{SpanID: NewSpanID(), TraceFlags: 1}
	var parentID string
	if parent.IsValid() {
		sc.TraceID = parent.TraceID
		sc.TraceFlags = parent.TraceFlags & 1
		sc.TraceState = parent.TraceState
		parentID = parent.SpanID
	} else {
		sc.TraceID = NewTraceID()
	}
	logger := slog.Default()
	span := &Span{
		sc:     sc,
		logger: logger,
		data: spanData{
			ParentSpanID: parentID,
			Name:         name,
			Kind:         SpanKindInternal,
			StartTime:    time.Now(),
		},
	}
	ctx = context.WithValue(ContextWithSpanContext(ctx, sc), spanKey{}, span)
	span.ctx = ctx
	span.enabled = sc.IsSampled() && logger.Enabled(ctx, slog.LevelInfo)
	if span.enabled {
		var pcs [1]uintptr
		// Skip runtime.Callers and StartSpan, retaining only the caller.
		runtime.Callers(2, pcs[:])
		span.pc = pcs[0]
	}
	span.SetAttrs(attrs...)
	return ctx, span
}

// SpanFromContext returns the local span in ctx if its SpanContext equals the
// context's current SpanContext; otherwise it returns nil. A nil ctx returns nil.
func SpanFromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	span, _ := ctx.Value(spanKey{}).(*Span)
	if span == nil || span.sc != SpanContextFromContext(ctx) {
		return nil
	}
	return span
}

// SpanContext returns the span's immutable correlation identity.
func (span *Span) SpanContext() SpanContext { return span.sc }

// IsRecording reports whether this span accepts attributes and events.
func (span *Span) IsRecording() bool {
	span.mu.Lock()
	defer span.mu.Unlock()
	return span.recording()
}

func (span *Span) recording() bool {
	return !span.ended && span.enabled
}

// SetAttrs adds attributes while the span is recording, preserving their order
// and duplicate keys. It resolves LogValuers when called and copies attribute
// slices and groups.
// Values held by slog.Any remain shallow references and must remain safe to
// read until the span is logged.
// Other log calls do not inherit these attributes; use [WithAttrs] for log metadata.
func (span *Span) SetAttrs(attrs ...slog.Attr) {
	if !span.IsRecording() {
		return
	}
	// Evaluate user LogValuers outside the span lock.
	resolved := appendSpanAttrs(nil, attrs)
	span.mu.Lock()
	defer span.mu.Unlock()
	if span.recording() {
		span.data.Attributes = append(span.data.Attributes, resolved...)
	}
}

// SetKind changes the operation kind. Values outside the declared kinds are
// ignored, as are changes to a span that is not recording.
func (span *Span) SetKind(kind SpanKind) {
	if kind < SpanKindInternal || kind > SpanKindConsumer {
		return
	}
	span.mu.Lock()
	defer span.mu.Unlock()
	if span.recording() {
		span.data.Kind = kind
	}
}

// SetStatus sets the operation outcome while the span is recording. The last
// valid call wins; description is retained only for StatusError. Recording an
// error does not set this status.
func (span *Span) SetStatus(status SpanStatus, description string) {
	if status > StatusError {
		return
	}
	span.mu.Lock()
	defer span.mu.Unlock()
	if span.recording() {
		span.data.Status = status
		span.data.StatusDescription = ""
		if status == StatusError {
			span.data.StatusDescription = description
		}
	}
}

// RecordError records an "error" event with a structured "err" attribute and
// any supplied attrs. Errors from [Wrap] retain their layered attributes.
// A nil error or a span that is not recording does nothing. This does not write
// a separate log record or change the span's outcome; call [Span.SetStatus] when
// the operation itself failed.
func (span *Span) RecordError(err error, attrs ...slog.Attr) {
	if err == nil || !span.IsRecording() {
		return
	}
	event := spanEvent{
		Time:       time.Now(),
		Name:       "error",
		Attributes: appendSpanAttrs(nil, []slog.Attr{slog.Any("err", err)}),
	}
	event.Attributes = appendSpanAttrs(event.Attributes, attrs)
	span.mu.Lock()
	defer span.mu.Unlock()
	if span.recording() {
		span.data.Events = append(span.data.Events, event)
	}
}

// End finishes the span and, if it is recording and the logger captured by
// [StartSpan] still enables INFO, logs it once. Request cancellation and deadlines
// are detached while context attributes are retained. Concurrent and repeated
// calls wait for that completion; later mutations do nothing. As with native
// slog methods, handler errors are ignored.
func (span *Span) End() {
	span.finish(nil)
}

// EndContext is like [Span.End], passing ctx to the captured logger. A nil ctx
// uses [context.Background]. Pass the context returned by [StartSpan] to retain
// its log attributes and correlation. The first End or EndContext call wins.
func (span *Span) EndContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	span.finish(ctx)
}

func (span *Span) finish(ctx context.Context) {
	span.once.Do(func() {
		span.mu.Lock()
		recording := span.recording()
		span.ended = true
		data := span.data
		data.EndTime = time.Now()
		logger := span.logger
		if recording && ctx == nil {
			ctx = context.WithoutCancel(span.ctx)
		}
		// A finished handle retains its identity, not request or recording state.
		span.data = spanData{}
		span.ctx = nil
		span.logger = nil
		span.mu.Unlock()
		if !recording || !logger.Enabled(ctx, slog.LevelInfo) {
			return
		}
		record := slog.NewRecord(data.EndTime, slog.LevelInfo, data.Name, span.pc)
		record.AddAttrs(data.Attributes...)
		record.AddAttrs(spanRecordAttr(span.sc, data))
		_ = logger.Handler().Handle(ctx, record)
	})
}

// spanRecordAttr describes an operation using the reserved "span" group.
// User attributes are siblings of that group; the handler applies WithGroup
// to all of them when the captured logger has an active group.
func spanRecordAttr(sc SpanContext, data spanData) slog.Attr {
	attrs := []slog.Attr{
		slog.String("trace_id", sc.TraceID),
		slog.String("span_id", sc.SpanID),
		slog.Int("trace_flags", int(sc.TraceFlags)),
		slog.String("name", data.Name),
		slog.Int("kind", int(data.Kind)),
		slog.Time("start_time", data.StartTime),
		slog.Time("end_time", data.EndTime),
		slog.Int("status", int(data.Status)),
	}
	if data.ParentSpanID != "" {
		attrs = append(attrs, slog.String("parent_span_id", data.ParentSpanID))
	}
	if sc.TraceState != "" {
		attrs = append(attrs, slog.String("trace_state", sc.TraceState))
	}
	if data.StatusDescription != "" {
		attrs = append(attrs, slog.String("status_message", data.StatusDescription))
	}
	var events []slog.Attr
	for i, event := range data.Events {
		events = append(events, slog.GroupAttrs(strconv.Itoa(i),
			slog.Time(slog.TimeKey, event.Time),
			slog.String(slog.MessageKey, event.Name),
			slog.GroupAttrs("attrs", event.Attributes...),
		))
	}
	if len(events) != 0 {
		attrs = append(attrs, slog.GroupAttrs("events", events...))
	}
	return slog.GroupAttrs("span", attrs...)
}

func appendSpanAttrs(dst, attrs []slog.Attr) []slog.Attr {
	for _, attr := range attrs {
		attr.Value = attr.Value.Resolve()
		if attr.Equal(slog.Attr{}) {
			continue
		}
		if attr.Value.Kind() == slog.KindGroup {
			group := appendSpanAttrs(nil, attr.Value.Group())
			if attr.Key == "" {
				dst = append(dst, group...)
				continue
			}
			if len(group) == 0 {
				continue
			}
			attr.Value = slog.GroupValue(group...)
		}
		dst = append(dst, attr)
	}
	return dst
}
