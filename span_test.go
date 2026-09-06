package slogx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rah-0/slogx"
)

type spanLogValueFunc func() slog.Value

func (value spanLogValueFunc) LogValue() slog.Value { return value() }

type spanErrorFunc func() string

func (err spanErrorFunc) Error() string { return err() }

type spanHandler struct {
	enabled func(context.Context, slog.Level) bool
	handle  func(context.Context, slog.Record) error
}

func (handler spanHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.enabled == nil || handler.enabled(ctx, level)
}
func (handler spanHandler) Handle(ctx context.Context, record slog.Record) error {
	return handler.handle(ctx, record)
}
func (handler spanHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }
func (handler spanHandler) WithGroup(string) slog.Handler      { return handler }

func TestStartSpanThreeLayersAndSiblingIsolation(t *testing.T) {
	type contextKey struct{}
	deadline := time.Now().Add(time.Hour)
	base, cancel := context.WithDeadline(context.WithValue(context.Background(), contextKey{}, "request"), deadline)
	t.Cleanup(cancel)
	var records []slog.Record
	setTestDefault(t, slog.New(spanHandler{handle: func(_ context.Context, record slog.Record) error {
		records = append(records, record.Clone())
		return nil
	}}))
	firstContext, first := slogx.StartSpan(base, "request", slog.Int("layer", 1))
	secondContext, second := slogx.StartSpan(firstContext, "load", slog.Int("layer", 2))
	thirdContext, third := slogx.StartSpan(secondContext, "query", slog.Int("layer", 3))
	siblingContext, sibling := slogx.StartSpan(firstContext, "cache", slog.Int("layer", 4))

	if slogx.SpanFromContext(base) != nil || slogx.SpanContextFromContext(base).IsValid() {
		t.Fatal("starting a span changed the original context")
	}
	identities := make(map[string]bool)
	for _, test := range []struct {
		ctx  context.Context
		span *slogx.Span
	}{{firstContext, first}, {secondContext, second}, {thirdContext, third}, {siblingContext, sibling}} {
		sc := test.span.SpanContext()
		if !sc.IsValid() || !sc.IsSampled() || sc.Remote || sc.TraceID != first.SpanContext().TraceID {
			t.Fatalf("span identity = %+v, want a local sampled span in the root trace", sc)
		}
		if identities[sc.SpanID] {
			t.Fatalf("duplicate span ID %q", sc.SpanID)
		}
		identities[sc.SpanID] = true
		if slogx.SpanFromContext(test.ctx) != test.span || slogx.SpanContextFromContext(test.ctx) != sc {
			t.Fatal("a derived context replaced its parent or sibling's span")
		}
		if test.ctx.Value(contextKey{}) != "request" || test.ctx.Done() != base.Done() {
			t.Fatal("starting a span lost context values or cancellation")
		}
		if actual, ok := test.ctx.Deadline(); !ok || actual != deadline {
			t.Fatalf("deadline = %v, %t, want %v", actual, ok, deadline)
		}
	}
	for _, span := range []*slogx.Span{third, second, sibling, first} {
		span.End()
		if span.IsRecording() {
			t.Fatal("finished span is recording")
		}
	}
	if len(records) != 4 {
		t.Fatalf("logged %d spans, want 4", len(records))
	}
	for _, record := range records {
		attrs := spanRecordAttrs(record)
		metadata := spanAttribute(t, attrs, "span").Group()
		var parentID string
		var layer int64
		switch record.Message {
		case "request":
			layer = 1
		case "load":
			parentID, layer = first.SpanContext().SpanID, 2
		case "query":
			parentID, layer = second.SpanContext().SpanID, 3
		case "cache":
			parentID, layer = first.SpanContext().SpanID, 4
		default:
			t.Fatalf("unexpected span %q", record.Message)
		}
		if got := optionalSpanAttribute(metadata, "parent_span_id"); (parentID == "" && got.Kind() != slog.KindAny) || (parentID != "" && got.String() != parentID) {
			t.Fatalf("parent ID = %v, want %q", got, parentID)
		}
		if spanAttribute(t, attrs, "layer").Int64() != layer || record.Level != slog.LevelInfo {
			t.Fatalf("span lost its own layer or level: %+v", record)
		}
		if spanAttribute(t, metadata, "name").String() != record.Message || spanAttribute(t, metadata, "trace_id").String() != first.SpanContext().TraceID {
			t.Fatalf("span lost name or trace identity: %v", metadata)
		}
		if spanAttribute(t, metadata, "kind").Int64() != int64(slogx.SpanKindInternal) || spanAttribute(t, metadata, "status").Int64() != 0 || spanAttribute(t, metadata, "trace_flags").Int64() != 1 {
			t.Fatalf("invalid default span metadata: %v", metadata)
		}
		start, end := spanAttribute(t, metadata, "start_time").Time(), spanAttribute(t, metadata, "end_time").Time()
		if start.IsZero() || end.Before(start) || !record.Time.Equal(end) {
			t.Fatalf("invalid operation timestamps: start=%v end=%v record=%v", start, end, record.Time)
		}
	}
	cancel()
	if !errors.Is(thirdContext.Err(), context.Canceled) {
		t.Fatalf("child context error = %v, want cancellation", thirdContext.Err())
	}
}

func TestSpanWithoutRecordingStillCorrelatesLogs(t *testing.T) {
	for _, unsampled := range []bool{false, true} {
		name := "disabled INFO level"
		if unsampled {
			name = "unsampled remote parent"
		}
		t.Run(name, func(t *testing.T) {
			setTestDefault(t, slog.New(spanHandler{
				enabled: func(context.Context, slog.Level) bool { return unsampled },
				handle: func(context.Context, slog.Record) error {
					t.Error("disabled span was logged")
					return nil
				},
			}))
			ctx := context.Background()
			remote := slogx.SpanContext{
				TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7",
				TraceState: "vendor=opaque", Remote: true,
			}
			if unsampled {
				ctx = slogx.ContextWithSpanContext(ctx, remote)
			}
			var evaluated atomic.Int32
			value := spanLogValueFunc(func() slog.Value {
				evaluated.Add(1)
				return slog.StringValue("evaluated")
			})
			ctx, span := slogx.StartSpan(ctx, "operation", slog.Any("initial", value))
			span.SetAttrs(slog.Any("later", value))
			span.RecordError(spanErrorFunc(func() string {
				evaluated.Add(1)
				return "failure"
			}), slog.Any("event", value))
			if span.IsRecording() || evaluated.Load() != 0 {
				t.Fatalf("disabled span recorded or evaluated values: recording=%t, evaluated=%d", span.IsRecording(), evaluated.Load())
			}
			sc := span.SpanContext()
			if !sc.IsValid() || sc.Remote {
				t.Fatalf("local identity = %+v", sc)
			}
			if unsampled && (sc.TraceID != remote.TraceID || sc.SpanID == remote.SpanID || sc.TraceState != remote.TraceState || sc.IsSampled()) {
				t.Fatalf("remote parent was not continued correctly: %+v", sc)
			}
			var output bytes.Buffer
			logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output})
			logger.InfoContext(ctx, "operation")
			record := decodeContextRecord(t, &output)
			if string(errorAttributeValue(t, record.Attributes, "trace_id")) != `"`+sc.TraceID+`"` || string(errorAttributeValue(t, record.Attributes, "span_id")) != `"`+sc.SpanID+`"` {
				t.Fatalf("log is missing correlation: %+v", record)
			}
			span.End()
		})
	}
}

func TestSpanFromContextDoesNotExposeReplacedAncestor(t *testing.T) {
	setTestDefault(t, slog.New(slog.DiscardHandler))
	ctx, span := slogx.StartSpan(context.Background(), "parent")
	defer span.End()
	replacement := span.SpanContext()
	replacement.SpanID = slogx.NewSpanID()
	replacement.Remote = true
	for _, other := range []context.Context{nil, context.Background(), slogx.ContextWithSpanContext(ctx, replacement), slogx.ContextWithSpanContext(ctx, slogx.SpanContext{})} {
		if slogx.SpanFromContext(other) != nil {
			t.Fatal("context without the current local identity exposed an ancestor span")
		}
	}
}

func TestSpanAttributesSnapshotAndResolveOutsideLocks(t *testing.T) {
	var record slog.Record
	var span *slogx.Span
	setTestDefault(t, slog.New(spanHandler{handle: func(_ context.Context, got slog.Record) error {
		if span.IsRecording() {
			t.Error("span still records while logging")
		}
		span.SetAttrs(slog.String("after_end", "ignored"))
		record = got.Clone()
		return nil
	}}))
	leaf := []slog.Attr{slog.String("value", "original")}
	group := []slog.Attr{slog.GroupAttrs("inner", leaf...)}
	attrs := []slog.Attr{slog.String("duplicate", "first"), slog.GroupAttrs("outer", group...)}
	_, span = slogx.StartSpan(t.Context(), "operation", attrs...)
	attrs[0] = slog.String("duplicate", "changed input")
	leaf[0] = slog.String("value", "changed input")
	group[0] = slog.String("inner", "changed input")
	shared := &struct{ Count int }{Count: 1}
	evaluated := 0
	span.SetAttrs(
		slog.String("duplicate", "second"), slog.String("duplicate", "last"),
		slog.Group("", slog.Int("inline", 42)),
		slog.Any("shared", shared),
		slog.Any("resolved", spanLogValueFunc(func() slog.Value {
			evaluated++
			span.SetKind(slogx.SpanKindServer)
			return slog.GroupValue(slog.String("value", "resolved"))
		})),
	)
	shared.Count = 2
	span.End()
	got := spanRecordAttrs(record)
	if evaluated != 1 || spanAttribute(t, spanAttribute(t, got, "span").Group(), "kind").Int64() != int64(slogx.SpanKindServer) || len(got) != 8 {
		t.Fatalf("attributes were not resolved or retained correctly: evaluated=%d, attrs=%v", evaluated, got)
	}
	for i, key := range []string{"duplicate", "outer", "duplicate", "duplicate", "inline", "shared", "resolved", "span"} {
		if got[i].Key != key {
			t.Fatalf("attribute %d = %v, want key %q", i, got[i], key)
		}
	}
	for index, value := range map[int]string{0: "first", 2: "second", 3: "last"} {
		if !got[index].Equal(slog.String("duplicate", value)) {
			t.Fatalf("duplicate attribute %d = %v, want %q", index, got[index], value)
		}
	}
	if spanAttribute(t, got, "inline").Int64() != 42 {
		t.Fatalf("inline group failed: %v", got)
	}
	outer := spanAttribute(t, got, "outer").Group()
	inner := spanAttribute(t, outer, "inner").Group()
	if spanAttribute(t, inner, "value").String() != "original" {
		t.Fatalf("input group mutation changed span attributes: %v", got)
	}
	if spanAttribute(t, spanAttribute(t, got, "resolved").Group(), "value").String() != "resolved" {
		t.Fatal("resolved LogValuer lost its group")
	}
	if actual := spanAttribute(t, got, "shared").Any(); actual != shared || shared.Count != 2 {
		t.Fatalf("arbitrary values should retain their documented shallow reference: %v", actual)
	}
}

func TestSpanRecordErrorRetainsLayersAndKeepsStatusSeparate(t *testing.T) {
	err := slogx.Wrap(errors.New("connection refused"), "query item", slog.Group("db", "item_id", 42))
	err = slogx.Wrap(err, "load item", "cache_hit", false)
	err = slogx.Wrap(err, "handle request", slog.Group("http", "method", "GET"))
	for _, test := range []struct {
		name   string
		set    func(*slogx.Span)
		status slogx.SpanStatus
		desc   string
	}{
		{name: "recording alone"},
		{name: "failed", set: func(span *slogx.Span) { span.SetStatus(slogx.StatusError, "request failed") }, status: slogx.StatusError, desc: "request failed"},
		{name: "recovered", set: func(span *slogx.Span) {
			span.SetStatus(slogx.StatusError, "temporary failure")
			span.SetStatus(slogx.StatusOK, "discarded")
		}, status: slogx.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			setTestDefault(t, slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output}))
			_, span := slogx.StartSpan(t.Context(), "request")
			span.RecordError(nil)
			span.RecordError(err, slog.Group("retry", "attempt", 2))
			if test.set != nil {
				test.set(span)
			}
			span.SetStatus(slogx.SpanStatus(255), "invalid")
			span.SetKind(slogx.SpanKind(0))
			span.SetKind(slogx.SpanKind(255))
			span.End()
			var record struct {
				Span struct {
					Status      slogx.SpanStatus `json:"status"`
					Description string           `json:"status_message"`
					StartTime   time.Time        `json:"start_time"`
					EndTime     time.Time        `json:"end_time"`
					Events      map[string]struct {
						Time  time.Time       `json:"time"`
						Msg   string          `json:"msg"`
						Attrs json.RawMessage `json:"attrs"`
					} `json:"events"`
				} `json:"span"`
			}
			if decodeErr := json.Unmarshal(output.Bytes(), &record); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			data := record.Span
			if data.Status != test.status || data.Description != test.desc || len(data.Events) != 1 {
				t.Fatalf("error event changed unrelated span state: %+v", data)
			}
			event := data.Events["0"]
			if event.Msg != "error" || event.Time.Before(data.StartTime) || event.Time.After(data.EndTime) {
				t.Fatalf("invalid event metadata: %+v", event)
			}
			want := `{"err":{"msg":"handle request","attrs":{"http":{"method":"GET"}},"cause":{"msg":"load item","attrs":{"cache_hit":false},"cause":{"msg":"query item","attrs":{"db":{"item_id":42}},"cause":"connection refused"}}},"retry":{"attempt":2}}`
			if string(event.Attrs) != want {
				t.Fatalf("error event attributes = %s, want %s", event.Attrs, want)
			}
		})
	}
}

func TestSpanLogCorrelationKeepsSpanAttributesSeparate(t *testing.T) {
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output}).With("service", "catalog").WithGroup("request")
	setTestDefault(t, logger)
	ctx := slogx.WithAttrs(context.Background(), slog.String("request_id", "req-1"))
	rootContext, root := slogx.StartSpan(ctx, "request", slog.Bool("span_only", true))
	childContext, child := slogx.StartSpan(rootContext, "load", slog.Int("child_only", 42))
	logger.InfoContext(rootContext, "started")
	err := slogx.Wrap(errors.New("connection refused"), "load item", slog.Group("db", "item_id", 42))
	slogx.ErrorContext(childContext, "failed", err)
	root.End()
	child.End()
	decoder := json.NewDecoder(&output)
	for i, span := range []*slogx.Span{root, child, root, child} {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		fields, ok := record["request"].(map[string]any)
		if !ok || record["service"] != "catalog" || fields["request_id"] != "req-1" || fields["trace_id"] != span.SpanContext().TraceID || fields["span_id"] != span.SpanContext().SpanID {
			t.Fatalf("log %d lost grouped correlation or request attributes: %v", i, record)
		}
		for _, key := range []string{"trace_id", "span_id", "span"} {
			if _, present := record[key]; present {
				t.Fatalf("automatic %s escaped the logger group", key)
			}
		}
		if i < 2 {
			for _, key := range []string{"span_only", "child_only", "span"} {
				if _, present := fields[key]; present {
					t.Fatalf("span attribute %s was implicitly added to logs", key)
				}
			}
		} else if _, present := fields["span"]; !present {
			t.Fatalf("completion lost span metadata: %v", fields)
		}
	}
}

func TestSpanConcurrentEndLogsOnceAndIgnoresHandlerFailure(t *testing.T) {
	var calls atomic.Int32
	var span *slogx.Span
	setTestDefault(t, slog.New(spanHandler{handle: func(ctx context.Context, record slog.Record) error {
		calls.Add(1)
		if span.IsRecording() || record.Message != "operation" || slogx.SpanFromContext(ctx) != span {
			t.Error("handler did not receive the finished current span")
		}
		span.SetAttrs(slog.String("after_end", "ignored"))
		return errors.New("write failed")
	}}))
	_, span = slogx.StartSpan(t.Context(), "operation")
	var workers sync.WaitGroup
	for i := range 24 {
		workers.Go(func() {
			for range 8 {
				span.SetAttrs(slog.Int("worker", i))
				span.RecordError(errors.New("temporary failure"))
				span.SetKind(slogx.SpanKindClient)
				span.SetStatus(slogx.StatusError, "temporary failure")
				_ = span.IsRecording()
				_ = span.SpanContext()
			}
			span.End()
		})
	}
	workers.Wait()
	span.End()
	if calls.Load() != 1 || span.IsRecording() {
		t.Fatalf("handler calls = %d, recording = %t", calls.Load(), span.IsRecording())
	}
}

func TestSpanEndWaitsForInFlightLog(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var calls atomic.Int32
	setTestDefault(t, slog.New(spanHandler{handle: func(context.Context, slog.Record) error {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return nil
	}}))
	_, span := slogx.StartSpan(t.Context(), "operation")
	const waiters = 8
	results := make(chan struct{}, waiters+1)
	go func() { span.End(); results <- struct{}{} }()
	<-entered
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	started := make(chan struct{})
	for i := range waiters {
		go func() {
			started <- struct{}{}
			if i%2 == 0 {
				span.EndContext(canceled)
			} else {
				span.End()
			}
			results <- struct{}{}
		}()
	}
	for range waiters {
		<-started
	}
	select {
	case <-results:
		t.Fatal("End returned before the handler completed")
	default:
	}
	unblock()
	for range waiters + 1 {
		<-results
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestSpanEndContextContract(t *testing.T) {
	type contextKey struct{}
	for _, mode := range []string{"detached", "explicit", "nil"} {
		t.Run(mode, func(t *testing.T) {
			base, cancel := context.WithDeadline(context.WithValue(context.Background(), contextKey{}, "request"), time.Now().Add(time.Hour))
			t.Cleanup(cancel)
			explicit, explicitCancel := context.WithCancel(context.Background())
			explicitCancel()
			var got context.Context
			setTestDefault(t, slog.New(spanHandler{handle: func(ctx context.Context, _ slog.Record) error {
				got = ctx
				return nil
			}}))
			_, span := slogx.StartSpan(base, "operation")
			cancel()
			switch mode {
			case "detached":
				span.End()
				_, deadline := got.Deadline()
				if got.Err() != nil || got.Done() != nil || deadline || got.Value(contextKey{}) != "request" {
					t.Fatal("End did not detach cancellation and deadline while retaining values")
				}
			case "explicit":
				span.EndContext(explicit)
				if got != explicit {
					t.Fatal("EndContext did not use the explicit canceled context")
				}
			case "nil":
				span.EndContext(nil)
				if got != context.Background() {
					t.Fatal("nil EndContext did not use Background")
				}
			}
		})
	}
}

func TestSpanEndRechecksLevel(t *testing.T) {
	var output bytes.Buffer
	var level slog.LevelVar
	setTestDefault(t, slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: &level})))
	_, span := slogx.StartSpan(t.Context(), "operation")
	if !span.IsRecording() {
		t.Fatal("INFO enabled span did not record")
	}
	level.Set(slog.LevelError)
	span.End()
	if output.Len() != 0 || span.IsRecording() {
		t.Fatal("End ignored the current level or did not finish")
	}
}

func spanRecordAttrs(record slog.Record) []slog.Attr {
	var attrs []slog.Attr
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	return attrs
}

func spanAttribute(t *testing.T, attrs []slog.Attr, key string) slog.Value {
	t.Helper()
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value
		}
	}
	t.Fatalf("attribute %q missing from %v", key, attrs)
	return slog.Value{}
}

func optionalSpanAttribute(attrs []slog.Attr, key string) slog.Value {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value
		}
	}
	return slog.Value{}
}

func TestSpanAttributesPreserveNativeDuplicateOrder(t *testing.T) {
	initial := []slog.Attr{
		slog.String("stage", "start"), slog.String("stage", "ready"),
		slog.Group("item", "id", 1, "id", 2,
			slog.Group("details", "value", "first"),
			slog.Group("details", "value", "second")),
		slog.Attr{}, slog.Group("empty"),
	}
	updates := [][]slog.Attr{
		{
			slog.String("stage", "loading"),
			slog.Group("item", "id", 3),
			slog.Group("", slog.String("stage", "inline"), slog.Group("item", "id", 4)),
		},
		{
			slog.String("stage", "done"),
			slog.Group("item", slog.Group("details", "value", "third"),
				slog.Group("", slog.String("id", "final"), slog.String("id", "complete"))),
		},
	}
	var record slog.Record
	setTestDefault(t, slog.New(spanHandler{handle: func(_ context.Context, got slog.Record) error {
		record = got.Clone()
		return nil
	}}))
	_, span := slogx.StartSpan(t.Context(), "operation", initial...)
	want := append([]slog.Attr(nil), initial...)
	for _, attrs := range updates {
		span.SetAttrs(attrs...)
		want = append(want, attrs...)
	}
	span.End()
	for _, format := range []slogx.Format{slogx.JSON, slogx.Text} {
		t.Run(formatName(format), func(t *testing.T) {
			attrs := spanRecordAttrs(record)
			actual := renderSpanAttrs(t, format, attrs[:len(attrs)-1])
			if expected := renderSpanAttrs(t, format, want); actual != expected {
				t.Fatalf("span attributes = %s, want native output %s", actual, expected)
			}
		})
	}
}

func TestSpanRecordErrorPreservesNativeDuplicateOrder(t *testing.T) {
	err := slogx.Wrap(errors.New("connection refused"), "query item",
		"attempt", 1, "attempt", 2,
		slog.Group("db", "host", "first"), slog.Group("db", "host", "second"))
	attrs := []slog.Attr{
		slog.Any("err", errors.New("caller detail")),
		slog.Int("attempt", 3), slog.Int("attempt", 4),
		slog.Group("retry", "delay_ms", 5),
		slog.Group("retry", "delay_ms", 10, "delay_ms", 20),
		slog.Group("", slog.Int("attempt", 5), slog.Group("retry", "delay_ms", 30)),
	}
	var record slog.Record
	setTestDefault(t, slog.New(spanHandler{handle: func(_ context.Context, got slog.Record) error {
		record = got.Clone()
		return nil
	}}))
	_, span := slogx.StartSpan(t.Context(), "operation")
	span.RecordError(err, attrs...)
	span.End()
	metadata := spanAttribute(t, spanRecordAttrs(record), "span").Group()
	events := spanAttribute(t, metadata, "events").Group()
	if len(events) != 1 || spanAttribute(t, events[0].Value.Group(), "msg").String() != "error" {
		t.Fatalf("events = %+v, want one error event", events)
	}
	eventAttrs := spanAttribute(t, events[0].Value.Group(), "attrs").Group()
	want := append([]slog.Attr{slog.Any("err", err)}, attrs...)
	for _, format := range []slogx.Format{slogx.JSON, slogx.Text} {
		t.Run(formatName(format), func(t *testing.T) {
			actual := renderSpanAttrs(t, format, eventAttrs)
			if expected := renderSpanAttrs(t, format, want); actual != expected {
				t.Fatalf("event attributes = %s, want native output %s", actual, expected)
			}
		})
	}
}

func renderSpanAttrs(t *testing.T, format slogx.Format, attrs []slog.Attr) string {
	t.Helper()
	var output bytes.Buffer
	var handler slog.Handler = slog.NewTextHandler(&output, nil)
	if format == slogx.JSON {
		handler = slog.NewJSONHandler(&output, nil)
	}
	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "operation", 0)
	record.AddAttrs(attrs...)
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestSpanSourceCapturesThreeLayerCallers(t *testing.T) {
	var output bytes.Buffer
	setTestDefault(t, slogx.New(slogx.Options{Format: slogx.JSON, AddSource: true, Writer: &output}))
	expected := make(map[string]slog.Source)
	spanSourceRequest(t, t.Context(), expected)
	decoder := slogx.NewJSONDecoder(&output)
	for range 3 {
		record, err := decoder.Decode()
		if err != nil {
			t.Fatal(err)
		}
		want, ok := expected[record.Message]
		if !ok {
			t.Fatalf("unexpected span %q", record.Message)
		}
		assertErrorSource(t, record.Source, want)
		delete(expected, record.Message)
	}
	if len(expected) != 0 {
		t.Fatalf("missing span sources: %v", expected)
	}
	if _, err := decoder.Decode(); !errors.Is(err, io.EOF) {
		t.Fatalf("extra record: %v", err)
	}
}

func spanSourceRequest(t *testing.T, ctx context.Context, expected map[string]slog.Source) {
	ctx, span := slogx.StartSpan(ctx, "request")
	expected["request"] = sourceImmediatelyBefore(t)
	spanSourceLoad(t, ctx, expected)
	span.End()
}

func spanSourceLoad(t *testing.T, ctx context.Context, expected map[string]slog.Source) {
	ctx, span := slogx.StartSpan(ctx, "load")
	expected["load"] = sourceImmediatelyBefore(t)
	spanSourceQuery(t, ctx, expected)
	span.End()
}

func spanSourceQuery(t *testing.T, ctx context.Context, expected map[string]slog.Source) {
	_, span := slogx.StartSpan(ctx, "query")
	expected["query"] = sourceImmediatelyBefore(t)
	span.End()
}

func TestSpanCapturesLoggerAtStart(t *testing.T) {
	for _, test := range []struct {
		name      string
		addSource bool
		derived   bool
		native    bool
	}{
		{name: "slogx without source"},
		{name: "slogx with source", addSource: true},
		{name: "derived logger with source", addSource: true, derived: true},
		{name: "native logger with source", addSource: true, native: true},
		{name: "native logger without source", native: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output, laterOutput bytes.Buffer
			var logger *slog.Logger
			if test.native {
				logger = slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{AddSource: test.addSource}))
			} else {
				logger = slogx.New(slogx.Options{Format: slogx.JSON, AddSource: test.addSource, Writer: &output})
			}
			if test.derived {
				logger = logger.With("service", "catalog").WithGroup("request").With("request_id", "req-1")
			}
			setTestDefault(t, logger)
			_, span := slogx.StartSpan(t.Context(), "operation")
			expected := sourceImmediatelyBefore(t)
			slog.SetDefault(slogx.New(slogx.Options{Format: slogx.JSON, AddSource: !test.addSource, Writer: &laterOutput}))
			span.End()
			if laterOutput.Len() != 0 {
				t.Fatal("span used the logger installed after StartSpan")
			}
			record := decodeContextRecord(t, &output)
			if test.addSource {
				assertErrorSource(t, record.Source, expected)
			} else if record.Source != nil {
				t.Fatalf("source = %+v, want none", record.Source)
			}
		})
	}
}

func TestSpanSourceMatchesNativeHandlerOutput(t *testing.T) {
	var record slog.Record
	setTestDefault(t, slog.New(spanHandler{handle: func(_ context.Context, got slog.Record) error {
		record = got.Clone()
		return nil
	}}))
	var pcs [1]uintptr
	// The native record and StartSpan must refer to the same caller line.
	if _, span := slogx.StartSpan(t.Context(), "operation"); runtime.Callers(1, pcs[:]) == 1 {
		span.End()
	} else {
		t.Fatal("resolve caller PC")
	}
	native := slog.NewRecord(record.Time, record.Level, record.Message, pcs[0])
	record.Attrs(func(attr slog.Attr) bool {
		native.AddAttrs(attr)
		return true
	})
	for _, format := range []slogx.Format{slogx.JSON, slogx.Text} {
		t.Run(formatName(format), func(t *testing.T) {
			var actualOutput, nativeOutput bytes.Buffer
			options := &slog.HandlerOptions{AddSource: true}
			actualHandler := nativeTraceHandler(format, &actualOutput, options)
			nativeHandler := nativeTraceHandler(format, &nativeOutput, options)
			if err := actualHandler.Handle(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			if err := nativeHandler.Handle(t.Context(), native); err != nil {
				t.Fatal(err)
			}
			if actualOutput.String() != nativeOutput.String() {
				t.Fatalf("span output = %q, want native output %q", actualOutput.String(), nativeOutput.String())
			}
		})
	}
}

func TestSpanReplaceAttrReachesSourceContextAndNestedErrors(t *testing.T) {
	for _, addSource := range []bool{false, true} {
		name := "without source"
		if addSource {
			name = "with source"
		}
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			seen := make(map[string]bool)
			setTestDefault(t, slogx.New(slogx.Options{
				Format: slogx.JSON, Writer: &output, AddSource: addSource,
				ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
					path := strings.Join(append(append([]string(nil), groups...), attr.Key), ".")
					seen[path] = true
					if attr.Key == "secret" {
						return slog.String("secret", "[redacted]")
					}
					if len(groups) == 0 && attr.Key == slog.SourceKey {
						if source, ok := attr.Value.Any().(*slog.Source); !ok || source == nil {
							t.Errorf("source value = %v, want native *slog.Source", attr.Value)
						}
						attr.Key = "caller"
					}
					return attr
				},
			}))
			ctx := slogx.WithAttrs(t.Context(), slog.Group("request", "secret", "context-secret"))
			_, span := slogx.StartSpan(ctx, "operation", slog.Group("operation", "secret", "span-secret"))
			err := slogx.Wrap(errors.New("failure"), "inner", slog.Group("details", "secret", "error-secret"))
			span.RecordError(err, slog.Group("retry", "secret", "event-secret"))
			span.End()
			for _, path := range []string{"request.secret", "operation.secret", "span.events.0.attrs.err.attrs.details.secret", "span.events.0.attrs.retry.secret"} {
				if !seen[path] {
					t.Errorf("ReplaceAttr did not visit %s", path)
				}
			}
			if seen[slog.SourceKey] != addSource {
				t.Fatalf("source callback = %t, AddSource=%t", seen[slog.SourceKey], addSource)
			}
			for _, secret := range []string{"context-secret", "span-secret", "error-secret", "event-secret"} {
				if strings.Contains(output.String(), secret) {
					t.Errorf("output retains %q: %s", secret, &output)
				}
			}
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if _, present := record["caller"]; present != addSource {
				t.Fatalf("renamed source presence=%t, AddSource=%t", present, addSource)
			}
		})
	}
}

func TestSpanEndReleasesOwnedState(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, test := range []struct {
		name string
		end  func(*slogx.Span)
	}{
		{name: "End", end: (*slogx.Span).End},
		{name: "EndContext", end: func(span *slogx.Span) { span.EndContext(context.Background()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = slogx.WithAttrs(ctx, slog.String("request_id", "req-1"))
			_, span := slogx.StartSpan(ctx, "operation", slog.String("attribute", "value"))
			span.SetStatus(slogx.StatusError, "operation failed")
			span.RecordError(errors.New("failure"))
			identity := span.SpanContext()
			test.end(span)

			// A caller may retain a finished span for its identity. It must not
			// retain the request context, logger, or completed recording too.
			state := reflect.ValueOf(span).Elem()
			if !state.FieldByName("ctx").IsNil() || !state.FieldByName("logger").IsNil() || !state.FieldByName("data").IsZero() {
				t.Fatal("finished span retains request or recording state")
			}
			if span.SpanContext() != identity {
				t.Fatal("releasing recording state changed the span identity")
			}
		})
	}
}
