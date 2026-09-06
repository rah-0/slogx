package slogx_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rah-0/slogx"
)

func TestWithAttrsSnapshotsParentsAndSiblings(t *testing.T) {
	attrs := make([]slog.Attr, 1, 4)
	attrs[0] = slog.String("request_id", "req-1")
	parent := slogx.WithAttrs(context.Background(), attrs...)
	attrs[0] = slog.String("request_id", "changed")
	childAttrs := []slog.Attr{slog.String("operation", "read")}
	child := slogx.WithAttrs(parent, childAttrs...)
	childAttrs[0] = slog.String("operation", "changed")
	sibling := slogx.WithAttrs(parent, slog.String("operation", "write"))

	for _, test := range []struct {
		name      string
		ctx       context.Context
		operation string
	}{
		{name: "parent", ctx: parent},
		{name: "child", ctx: child, operation: `"read"`},
		{name: "sibling", ctx: sibling, operation: `"write"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output})
			logger.InfoContext(test.ctx, "request")
			record := decodeContextRecord(t, &output)
			if actual := string(errorAttributeValue(t, record.Attributes, "request_id")); actual != `"req-1"` {
				t.Fatalf("request_id = %s, want req-1", actual)
			}
			if test.operation == "" {
				if len(record.Attributes) != 1 {
					t.Fatalf("parent attributes = %v, want request_id only", record.Attributes)
				}
				return
			}
			if actual := string(errorAttributeValue(t, record.Attributes, "operation")); actual != test.operation {
				t.Fatalf("operation = %s, want %s", actual, test.operation)
			}
		})
	}
}

func TestWithAttrsNilContext(t *testing.T) {
	if ctx := slogx.WithAttrs(nil); ctx != context.Background() {
		t.Fatalf("empty attributes context = %v, want Background", ctx)
	}
	ctx := slogx.WithAttrs(nil, slog.String("request_id", "req-1"))
	var output bytes.Buffer
	slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output}).InfoContext(ctx, "request")
	record := decodeContextRecord(t, &output)
	if actual := string(errorAttributeValue(t, record.Attributes, "request_id")); actual != `"req-1"` {
		t.Fatalf("request_id = %s, want req-1", actual)
	}
}

func TestWithAttrsPreservesContext(t *testing.T) {
	type key struct{}
	deadline := time.Now().Add(time.Hour)
	parent, cancel := context.WithDeadline(context.WithValue(context.Background(), key{}, "private"), deadline)
	t.Cleanup(cancel)
	ctx := slogx.WithAttrs(parent, slog.String("request_id", "req-1"))
	if slogx.WithAttrs(ctx) != ctx {
		t.Fatal("empty attributes replaced the context")
	}
	if actual := ctx.Value(key{}); actual != "private" {
		t.Fatalf("context value = %v, want private", actual)
	}
	if actual, ok := ctx.Deadline(); !ok || !actual.Equal(deadline) {
		t.Fatalf("deadline = %v, %t, want %v, true", actual, ok, deadline)
	}
	if ctx.Done() != parent.Done() {
		t.Fatal("cancellation channel changed")
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v, want cancellation", ctx.Err())
	}

	var output bytes.Buffer
	logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output})
	logger.InfoContext(ctx, "canceled request")
	record := decodeContextRecord(t, &output)
	if len(record.Attributes) != 1 || record.Attributes[0].Key != "request_id" {
		t.Fatalf("attributes = %v, want only explicit request_id", record.Attributes)
	}
}

func TestContextAttributesPreserveGroupsAndDuplicateOrder(t *testing.T) {
	ctx := slogx.WithAttrs(context.Background(), slog.String("duplicate", "parent"))
	ctx = slogx.WithAttrs(ctx,
		slog.String("duplicate", "child"),
		slog.Group("http", slog.String("method", "GET")),
	)
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output}).
		With("service", "catalog").
		WithGroup("request").
		With("static", "value").
		WithGroup("operation").
		With("duplicate", "logger")
	logger.InfoContext(ctx, "request", "duplicate", "call")
	record := decodeContextRecord(t, &output)
	if actual := string(errorAttributeValue(t, record.Attributes, "service")); actual != `"catalog"` {
		t.Fatalf("service = %s, want catalog", actual)
	}
	want := `{"static":"value","operation":{"duplicate":"logger","duplicate":"parent","duplicate":"child","http":{"method":"GET"},"duplicate":"call"}}`
	if actual := string(errorAttributeValue(t, record.Attributes, "request")); actual != want {
		t.Fatalf("request = %s, want %s", actual, want)
	}
}

func TestErrorContextIncludesContextAttributes(t *testing.T) {
	var output bytes.Buffer
	setTestDefault(t, slogx.NewDefault(slogx.Options{Format: slogx.JSON, Writer: &output, AddSource: true}))
	ctx := slogx.WithAttrs(context.Background(), slog.String("request_id", "req-1"))
	slogx.ErrorContext(ctx, "request failed", errors.New("connection refused"), "attempt", 2)
	expectedSource := sourceImmediatelyBefore(t)

	record := decodeContextRecord(t, &output)
	assertErrorSource(t, record.Source, expectedSource)
	if record.Level != slog.LevelError || record.Message != "request failed" {
		t.Fatalf("record = %+v, want ERROR request failed", record)
	}
	for i, expected := range []struct{ key, value string }{
		{key: "request_id", value: `"req-1"`},
		{key: "err", value: `"connection refused"`},
		{key: "attempt", value: "2"},
	} {
		if len(record.Attributes) <= i || record.Attributes[i].Key != expected.key || string(record.Attributes[i].Value) != expected.value {
			t.Fatalf("attributes = %v, want attribute %d to be %s=%s", record.Attributes, i, expected.key, expected.value)
		}
	}
}

func TestContextHandlerPreservesRecord(t *testing.T) {
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])
	timestamp := testTimestamp(123456789)
	record := slog.NewRecord(timestamp, slog.LevelWarn, "request", pcs[0])
	for i := range 8 {
		record.AddAttrs(slog.Int("attempt", i))
	}
	ctx := slogx.WithAttrs(context.Background(), slog.String("request_id", "req-1"))
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output, AddSource: true})
	if err := logger.Handler().Handle(ctx, record); err != nil {
		t.Fatalf("handle record: %v", err)
	}
	if record.NumAttrs() != 8 {
		t.Fatalf("original record has %d attrs, want 8", record.NumAttrs())
	}
	actual := decodeContextRecord(t, &output)
	if actual.Time != timestamp.Format(time.RFC3339Nano) || actual.Level != slog.LevelWarn || actual.Message != "request" {
		t.Fatalf("record metadata = %+v, want original timestamp, level and message", actual)
	}
	assertErrorSource(t, actual.Source, *record.Source())
	if len(actual.Attributes) != 9 {
		t.Fatalf("attribute count = %d, want 9", len(actual.Attributes))
	}
}

func TestContextLoggingWithoutAttributes(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "nil"},
		{name: "background", ctx: context.Background()},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output})
			setTestDefault(t, logger)
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "direct", 0)
			if err := logger.Handler().Handle(test.ctx, record); err != nil {
				t.Fatalf("handle record: %v", err)
			}
			logger.InfoContext(test.ctx, "info")
			slogx.ErrorContext(test.ctx, "error", nil)
			decoder := slogx.NewJSONDecoder(&output)
			for _, message := range []string{"direct", "info", "error"} {
				actual, err := decoder.Decode()
				if err != nil || actual.Message != message {
					t.Fatalf("record = %+v, %v, want message %s", actual, err, message)
				}
			}
		})
	}
}

func TestContextAttributesConcurrentReuse(t *testing.T) {
	ctx := slogx.WithAttrs(context.Background(), slog.String("request_id", "req-1"))
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output})
	var workers sync.WaitGroup
	for i := range 24 {
		workers.Go(func() {
			child := slogx.WithAttrs(ctx, slog.Int("worker", i))
			logger.InfoContext(child, "request")
		})
	}
	workers.Wait()
	decoder := slogx.NewJSONDecoder(&output)
	seen := make(map[string]bool)
	for {
		record, err := decoder.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode concurrent record: %v", err)
		}
		if actual := string(errorAttributeValue(t, record.Attributes, "request_id")); actual != `"req-1"` {
			t.Fatalf("request_id = %s, want req-1", actual)
		}
		worker := string(errorAttributeValue(t, record.Attributes, "worker"))
		if seen[worker] {
			t.Fatalf("duplicate worker = %s", worker)
		}
		seen[worker] = true
	}
	if len(seen) != 24 {
		t.Fatalf("worker count = %d, want 24", len(seen))
	}
}

func decodeContextRecord(t *testing.T, output io.Reader) slogx.JSONRecord {
	t.Helper()
	record, err := slogx.NewJSONDecoder(output).Decode()
	if err != nil {
		t.Fatalf("decode JSON record: %v", err)
	}
	return record
}

func TestTraceCorrelationPreservesNativeGroups(t *testing.T) {
	for _, format := range []slogx.Format{slogx.JSON, slogx.Text} {
		for _, test := range []struct {
			name  string
			bind  func(*slog.Logger) *slog.Logger
			attrs []slog.Attr
		}{
			{
				name: "ungrouped",
				bind: func(logger *slog.Logger) *slog.Logger {
					return logger.With("root", "bound").WithGroup("")
				},
				attrs: []slog.Attr{slog.String("call", "value")},
			},
			{
				name: "explicit call group on ungrouped logger",
				bind: func(logger *slog.Logger) *slog.Logger {
					return logger.With("service", "catalog")
				},
				attrs: []slog.Attr{slog.Group("request", "method", "GET", "item_id", 42)},
			},
			{
				name: "empty groups",
				bind: func(logger *slog.Logger) *slog.Logger {
					return logger.WithGroup("outer").With(slog.Group("empty")).WithGroup("inner")
				},
			},
			{
				name: "bound outer empty inner",
				bind: func(logger *slog.Logger) *slog.Logger {
					return logger.WithGroup("outer").With("bound", true).WithGroup("inner")
				},
			},
			{
				name: "nested with duplicates",
				bind: func(logger *slog.Logger) *slog.Logger {
					return logger.With("root", "bound").WithGroup("outer").
						With("duplicate", "first").With("duplicate", "second").
						WithGroup("").WithGroup("inner").
						With("duplicate", "inner", "nested", slog.GroupValue(slog.String("key", "value")))
				},
				attrs: []slog.Attr{slog.String("duplicate", "call"), slog.Group("", slog.String("inline", "value"))},
			},
			{
				name: "same group name at distinct levels",
				bind: func(logger *slog.Logger) *slog.Logger {
					return logger.WithGroup("operation").With("layer", 1).
						WithGroup("operation").With("layer", 2)
				},
				attrs: []slog.Attr{slog.Int("layer", 3)},
			},
			{
				name: "empty and inline bound attributes",
				bind: func(logger *slog.Logger) *slog.Logger {
					return logger.WithGroup("outer").With(slog.Attr{}, slog.Group("empty"),
						slog.Group("", slog.String("inline", "bound")), slog.String("", "empty-key"))
				},
				attrs: []slog.Attr{slog.Group("details", slog.String("key", "value"))},
			},
			{
				name: "record with backing attributes",
				bind: func(logger *slog.Logger) *slog.Logger { return logger.WithGroup("operation") },
				attrs: []slog.Attr{
					slog.Int("a", 1), slog.Int("b", 2), slog.Int("c", 3), slog.Int("d", 4),
					slog.Int("e", 5), slog.Int("f", 6), slog.Int("g", 7), slog.Int("h", 8),
				},
			},
		} {
			for _, withContextAttrs := range []bool{false, true} {
				t.Run(fmt.Sprintf("%v/%s/contextattrs=%t", format, test.name, withContextAttrs), func(t *testing.T) {
					var nativeOutput, actualOutput bytes.Buffer
					native := test.bind(slog.New(nativeTraceHandler(format, &nativeOutput, &slog.HandlerOptions{AddSource: true})))
					actual := test.bind(slogx.New(slogx.Options{Format: format, Writer: &actualOutput, AddSource: true}))
					var pcs [1]uintptr
					runtime.Callers(1, pcs[:])
					record := slog.NewRecord(time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC), slog.LevelInfo, "request", pcs[0])
					record.AddAttrs(test.attrs...)
					for _, traced := range []bool{false, true} {
						ctx := context.Background()
						expected := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
						if traced {
							ctx = slogx.ContextWithSpanContext(ctx, slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID})
							expected.AddAttrs(slog.String("trace_id", w3cTraceID), slog.String("span_id", w3cSpanID))
						}
						if withContextAttrs {
							attrs := []slog.Attr{slog.String("request_id", "request-1"), slog.String("duplicate", "context")}
							ctx = slogx.WithAttrs(ctx, attrs...)
							expected.AddAttrs(attrs...)
						}
						expected.AddAttrs(test.attrs...)
						nativeOutput.Reset()
						actualOutput.Reset()
						if err := native.Handler().Handle(ctx, expected); err != nil {
							t.Fatal(err)
						}
						if err := actual.Handler().Handle(ctx, record); err != nil {
							t.Fatal(err)
						}
						if actualOutput.String() != nativeOutput.String() {
							t.Fatalf("traced=%t output = %s, want native %s", traced, &actualOutput, &nativeOutput)
						}
						if record.NumAttrs() != len(test.attrs) {
							t.Fatal("handler changed the input record's attribute count")
						}
						index := 0
						record.Attrs(func(attr slog.Attr) bool {
							if !attr.Equal(test.attrs[index]) {
								t.Fatalf("handler changed input attribute %d: %v", index, attr)
							}
							index++
							return true
						})
					}
				})
			}
		}
	}
}

func nativeTraceHandler(format slogx.Format, output io.Writer, options *slog.HandlerOptions) slog.Handler {
	if format == slogx.JSON {
		return slog.NewJSONHandler(output, options)
	}
	return slog.NewTextHandler(output, options)
}

type contextBindingValue struct {
	calls *atomic.Int64
	value *atomic.Int64
}

func (value contextBindingValue) LogValue() slog.Value {
	value.calls.Add(1)
	return slog.GroupValue(slog.Int64("value", value.value.Load()))
}

func TestTraceGroupsResolveBoundLogValuersOnce(t *testing.T) {
	for _, format := range []slogx.Format{slogx.JSON, slogx.Text} {
		t.Run(fmt.Sprint(format), func(t *testing.T) {
			var nativeCalls, actualCalls, value atomic.Int64
			value.Store(1)
			var nativeOutput, actualOutput bytes.Buffer
			bind := func(logger *slog.Logger, calls *atomic.Int64) *slog.Logger {
				return logger.WithGroup("operation").With(slog.Group("details",
					slog.Any("dynamic", contextBindingValue{calls: calls, value: &value})))
			}
			native := bind(slog.New(nativeTraceHandler(format, &nativeOutput, nil)), &nativeCalls)
			actual := bind(slogx.New(slogx.Options{Format: format, Writer: &actualOutput}), &actualCalls)
			if nativeCalls.Load() != 1 || actualCalls.Load() != 1 {
				t.Fatalf("binding resolutions: native=%d, actual=%d; want one each", nativeCalls.Load(), actualCalls.Load())
			}
			value.Store(2)
			for _, traced := range []bool{true, false, true} {
				ctx := context.Background()
				record := slog.NewRecord(time.Time{}, slog.LevelInfo, "request", 0)
				expected := record.Clone()
				if traced {
					ctx = slogx.ContextWithSpanContext(ctx, slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID})
					expected.AddAttrs(slog.String("trace_id", w3cTraceID), slog.String("span_id", w3cSpanID))
				}
				if err := native.Handler().Handle(ctx, expected); err != nil {
					t.Fatal(err)
				}
				if err := actual.Handler().Handle(ctx, record); err != nil {
					t.Fatal(err)
				}
			}
			if nativeCalls.Load() != 1 || actualCalls.Load() != 1 || actualOutput.String() != nativeOutput.String() {
				t.Fatalf("bound LogValuer differs from native handling: calls=%d/%d, output=%s, native=%s", actualCalls.Load(), nativeCalls.Load(), &actualOutput, &nativeOutput)
			}
		})
	}
}

func TestTraceGroupsDerivedLoggersAreIndependent(t *testing.T) {
	var nativeOutput, actualOutput bytes.Buffer
	native := slog.New(slog.NewJSONHandler(&nativeOutput, nil)).WithGroup("operation").With("parent", true)
	actual := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &actualOutput}).WithGroup("operation").With("parent", true)
	ctx := slogx.ContextWithSpanContext(context.Background(), slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID})
	child := func(parent *slog.Logger, worker int) *slog.Logger {
		return parent.With("worker", worker).WithGroup("child").With("bound", "value")
	}
	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "child", 0)
	record.AddAttrs(slog.Bool("call", true))
	expected := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	expected.AddAttrs(slog.String("trace_id", w3cTraceID), slog.String("span_id", w3cSpanID), slog.Bool("call", true))
	var workers sync.WaitGroup
	for i := range 32 {
		workers.Go(func() {
			if err := child(actual, i).Handler().Handle(ctx, record); err != nil {
				t.Error(err)
			}
		})
		if err := child(native, i).Handler().Handle(ctx, expected); err != nil {
			t.Fatal(err)
		}
	}
	workers.Wait()
	record = slog.NewRecord(time.Time{}, slog.LevelInfo, "parent", 0)
	expected = record.Clone()
	expected.AddAttrs(slog.String("trace_id", w3cTraceID), slog.String("span_id", w3cSpanID))
	if err := actual.Handler().Handle(ctx, record); err != nil {
		t.Fatal(err)
	}
	if err := native.Handler().Handle(ctx, expected); err != nil {
		t.Fatal(err)
	}
	// Scheduling may reorder child records; compare each complete native record.
	actualRecords := strings.Split(strings.TrimSuffix(actualOutput.String(), "\n"), "\n")
	nativeRecords := strings.Split(strings.TrimSuffix(nativeOutput.String(), "\n"), "\n")
	slices.Sort(actualRecords)
	slices.Sort(nativeRecords)
	if !slices.Equal(actualRecords, nativeRecords) {
		t.Fatalf("derived loggers differ from native records:\nactual: %v\nnative: %v", actualRecords, nativeRecords)
	}
}

func TestTraceGroupsPreserveDisabledLevels(t *testing.T) {
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output, Level: slog.LevelError}).WithGroup("operation")
	ctx := slogx.ContextWithSpanContext(context.Background(), slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID})
	var calls, value atomic.Int64
	logger.InfoContext(ctx, "disabled", "value", contextBindingValue{calls: &calls, value: &value})
	if calls.Load() != 0 || output.Len() != 0 {
		t.Fatal("disabled log evaluated attributes or wrote output")
	}
}
