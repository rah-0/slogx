package slogx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rah-0/slogx"
)

func TestNewUsesTimeLayout(t *testing.T) {
	timestamp := testTimestamp(123456789)
	const timeLayout = "2006-01-02 15:04:05.000000"
	const expected = "2026-09-03 20:45:12.123456"

	for _, format := range []slogx.Format{slogx.Text, slogx.JSON} {
		t.Run(formatName(format), func(t *testing.T) {
			output := renderRecord(t, format, timeLayout, timestamp)
			t.Logf("slogx output: %s", strings.TrimSpace(output))
			if actual := outputTime(t, format, output); actual != expected {
				t.Fatalf("time = %q, want %q", actual, expected)
			}
		})
	}
}

func TestNewPreservesNativeTimestampByDefault(t *testing.T) {
	timestamp := testTimestamp(123456789)

	var nativeOutput bytes.Buffer
	nativeHandler := slog.NewJSONHandler(&nativeOutput, nil)
	handleRecord(t, nativeHandler, timestamp)

	actual := renderRecord(t, slogx.JSON, "", timestamp)
	t.Logf("slogx output: %s", strings.TrimSpace(actual))
	if actualTime, nativeTime := outputTime(t, slogx.JSON, actual), outputTime(t, slogx.JSON, nativeOutput.String()); actualTime != nativeTime {
		t.Fatalf("time = %q, want native slog time %q", actualTime, nativeTime)
	}
}

func TestNewUnsupportedFormatUsesNativeText(t *testing.T) {
	var output, nativeOutput bytes.Buffer
	logger := slogx.New(slogx.Options{Format: slogx.Format(255), Writer: &output}).
		With("service", "catalog").WithGroup("request")
	native := slog.New(slog.NewTextHandler(&nativeOutput, nil)).
		With("service", "catalog").WithGroup("request")
	record := slog.NewRecord(testTimestamp(123456789), slog.LevelWarn, "request failed", 0)
	record.AddAttrs(slog.Int("status", 503), slog.Group("retry", "attempt", 2))
	for _, handler := range []slog.Handler{logger.Handler(), native.Handler()} {
		if err := handler.Handle(t.Context(), record); err != nil {
			t.Fatalf("handle record: %v", err)
		}
	}
	if actual, want := output.String(), nativeOutput.String(); actual != want {
		t.Fatalf("unsupported format output = %q, want native text %q", actual, want)
	}
}

func TestNewDefaultUsesPreferredTimeLayout(t *testing.T) {
	for _, test := range []struct {
		name       string
		timestamp  time.Time
		timeLayout string
		expected   string
	}{
		{name: "default", timestamp: testTimestamp(123456789), expected: "2026-09-03 20:45:12.123456"},
		{name: "zero microseconds", timestamp: testTimestamp(0), expected: "2026-09-03 20:45:12.000000"},
		{name: "override", timestamp: testTimestamp(123456789), timeLayout: "2006/01/02-15:04:05", expected: "2026/09/03-20:45:12"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slogx.NewDefault(slogx.Options{
				Format:     slogx.JSON,
				TimeLayout: test.timeLayout,
				Writer:     &output,
			})
			handleRecord(t, logger.Handler(), test.timestamp)
			t.Logf("slogx output: %s", strings.TrimSpace(output.String()))

			if actual := outputTime(t, slogx.JSON, output.String()); actual != test.expected {
				t.Fatalf("time = %q, want %q", actual, test.expected)
			}
		})
	}
}

func TestNewUsesStderr(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	logger := func() *slog.Logger {
		stderr := os.Stderr
		os.Stderr = writer
		defer func() { os.Stderr = stderr }()
		return slogx.New(slogx.Options{})
	}()

	handleRecord(t, logger.Handler(), testTimestamp(123456789))
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	t.Logf("slogx stderr: %s", strings.TrimSpace(string(output)))
	if len(output) == 0 {
		t.Fatal("stderr is empty")
	}
}

func TestNewDefaultUsesStdout(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	logger := func() *slog.Logger {
		stdout := os.Stdout
		os.Stdout = writer
		defer func() { os.Stdout = stdout }()
		return slogx.NewDefault(slogx.Options{})
	}()

	handleRecord(t, logger.Handler(), testTimestamp(123456789))
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	t.Logf("slogx stdout: %s", strings.TrimSpace(string(output)))
	if len(output) == 0 {
		t.Fatal("stdout is empty")
	}
}

func TestNewDefaultHonorsAddSource(t *testing.T) {
	for _, test := range []struct {
		name      string
		addSource bool
	}{
		{name: "disabled"},
		{name: "enabled", addSource: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slogx.NewDefault(slogx.Options{
				AddSource: test.addSource,
				Format:    slogx.JSON,
				Writer:    &output,
			})
			_, expectedFile, before, ok := runtime.Caller(0)
			if !ok {
				t.Fatal("resolve caller before log")
			}
			logger.Info("source probe")
			_, _, after, ok := runtime.Caller(0)
			if !ok {
				t.Fatal("resolve caller after log")
			}
			t.Logf("slogx output: %s", strings.TrimSpace(output.String()))

			record, err := slogx.NewJSONDecoder(&output).Decode()
			if err != nil {
				t.Fatalf("decode JSON output: %v", err)
			}
			if !test.addSource {
				if record.Source != nil {
					t.Fatalf("source = %+v, want nil", record.Source)
				}
				return
			}
			if record.Source == nil {
				t.Fatal("source is nil")
			}
			if record.Source.File != expectedFile {
				t.Fatalf("source file = %q, want %q", record.Source.File, expectedFile)
			}
			if record.Source.Line <= before || record.Source.Line >= after {
				t.Fatalf("source line = %d, want logger call between %d and %d", record.Source.Line, before, after)
			}
			if record.Source.Function == "" {
				t.Fatal("source function is empty")
			}
		})
	}
}

func TestNewSupportsDynamicLevel(t *testing.T) {
	var output bytes.Buffer
	var level slog.LevelVar
	level.Set(slog.LevelInfo)
	logger := slogx.New(slogx.Options{
		Level:  &level,
		Format: slogx.JSON,
		Writer: &output,
	})

	logger.Debug("hidden")
	if output.Len() != 0 {
		t.Fatalf("debug output before level change: %q", output.String())
	}

	level.Set(slog.LevelDebug)
	logger.Debug("visible")
	t.Logf("slogx output: %s", strings.TrimSpace(output.String()))
	if !strings.Contains(output.String(), `"msg":"visible"`) {
		t.Fatalf("debug output after level change: %q", output.String())
	}
}

func TestSetDefaultConfiguresSlogGlobal(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	var output bytes.Buffer
	slogx.SetDefault(slogx.Options{
		Format: slogx.JSON,
		Writer: &output,
	})

	slog.Info("global probe")
	t.Logf("slogx global output: %s", strings.TrimSpace(output.String()))

	record, err := slogx.NewJSONDecoder(&output).Decode()
	if err != nil {
		t.Fatalf("decode global output: %v", err)
	}
	if record.Message != "global probe" {
		t.Fatalf("message = %q, want global probe", record.Message)
	}
	if record.Source != nil {
		t.Fatalf("source = %+v, want nil", record.Source)
	}
}

func testTimestamp(nanoseconds int) time.Time {
	zone := time.FixedZone("UTC+02:30", 2*60*60+30*60)
	return time.Date(2026, 9, 3, 23, 15, 12, nanoseconds, zone)
}

func renderRecord(t *testing.T, format slogx.Format, timeLayout string, timestamp time.Time) string {
	t.Helper()

	var output bytes.Buffer
	logger := slogx.New(slogx.Options{
		Format:     format,
		TimeLayout: timeLayout,
		Writer:     &output,
	})
	handleRecord(t, logger.Handler(), timestamp)
	return output.String()
}

func handleRecord(t *testing.T, handler slog.Handler, timestamp time.Time) {
	t.Helper()

	record := slog.NewRecord(timestamp, slog.LevelInfo, "probe", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("handle record: %v", err)
	}
}

func outputTime(t *testing.T, format slogx.Format, output string) string {
	t.Helper()

	if format == slogx.JSON {
		var record struct {
			Time string `json:"time"`
		}
		if err := json.Unmarshal([]byte(output), &record); err != nil {
			t.Fatalf("decode JSON output: %v", err)
		}
		return record.Time
	}

	value, ok := strings.CutPrefix(output, `time="`)
	if ok {
		if value, _, ok = strings.Cut(value, `"`); ok {
			return value
		}
	}
	t.Fatalf("text output has no time field: %q", output)
	return ""
}

func formatName(format slogx.Format) string {
	if format == slogx.JSON {
		return "json"
	}
	return "text"
}

func TestReplaceAttrMatchesNativeHandlers(t *testing.T) {
	for _, format := range []slogx.Format{slogx.JSON, slogx.Text} {
		t.Run(formatName(format), func(t *testing.T) {
			var output, nativeOutput bytes.Buffer
			var calls, nativeCalls []string
			replace := func(calls *[]string) func([]string, slog.Attr) slog.Attr {
				return func(groups []string, attr slog.Attr) slog.Attr {
					*calls = append(*calls, fmt.Sprintf("%s/%s:%s", strings.Join(groups, "."), attr.Key, attr.Value.Kind()))
					switch attr.Key {
					case "secret":
						return slog.String(attr.Key, "[redacted]")
					case "discard":
						return slog.Attr{}
					case "operation":
						attr.Key = "action"
					}
					return attr
				}
			}
			value := "bound before logging"
			bind := func(logger *slog.Logger) *slog.Logger {
				return logger.With("bound", spanLogValueFunc(func() slog.Value { return slog.StringValue(value) })).
					WithGroup("request").With("secret", "sensitive bound", "discard", "removed bound")
			}
			logger := bind(slogx.New(slogx.Options{Format: format, Writer: &output, AddSource: true, ReplaceAttr: replace(&calls)}))
			native := bind(slog.New(nativeTraceHandler(format, &nativeOutput, &slog.HandlerOptions{AddSource: true, ReplaceAttr: replace(&nativeCalls)})))
			if len(calls) == 0 || !slices.Equal(calls, nativeCalls) {
				t.Fatalf("callbacks at binding = %v, want native %v", calls, nativeCalls)
			}
			value = "bound after logging"
			ctxAttrs := []slog.Attr{slog.String("secret", "sensitive context"), slog.String("operation", "context operation")}
			ctx := slogx.WithAttrs(t.Context(), ctxAttrs...)
			ctx = slogx.ContextWithSpanContext(ctx, slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID})
			callAttrs := []slog.Attr{
				slog.String("secret", "sensitive call"), slog.String("discard", "removed call"),
				slog.Group("detail", "operation", "call operation"),
			}
			var pcs [1]uintptr
			runtime.Callers(1, pcs[:])
			record := slog.NewRecord(testTimestamp(123456789), slog.LevelInfo, "request", pcs[0])
			expected := record.Clone()
			record.AddAttrs(callAttrs...)
			expected.AddAttrs(slog.String("trace_id", w3cTraceID), slog.String("span_id", w3cSpanID))
			expected.AddAttrs(ctxAttrs...)
			expected.AddAttrs(callAttrs...)
			for range 2 {
				if err := logger.Handler().Handle(ctx, record); err != nil {
					t.Fatal(err)
				}
				if err := native.Handler().Handle(ctx, expected); err != nil {
					t.Fatal(err)
				}
			}
			if !slices.Equal(calls, nativeCalls) || output.String() != nativeOutput.String() {
				t.Fatalf("callbacks/output differ from native: calls=%v, native=%v; output=%s, native=%s", calls, nativeCalls, &output, &nativeOutput)
			}
			if strings.Contains(output.String(), "sensitive") || strings.Contains(output.String(), "removed") || strings.Contains(output.String(), value) {
				t.Fatalf("replacement or native binding was lost: %s", &output)
			}
			beforeCalls, beforeBytes := len(calls), output.Len()
			logger.DebugContext(ctx, "disabled", "secret", "sensitive disabled")
			if len(calls) != beforeCalls || output.Len() != beforeBytes {
				t.Fatal("disabled log invoked ReplaceAttr or produced output")
			}
		})
	}
}

func TestReplaceAttrRunsBeforeTimeLayout(t *testing.T) {
	timestamp := testTimestamp(123456789)
	const layout = "2006-01-02 15:04:05.000000"
	for _, test := range []struct {
		name    string
		replace func(slog.Attr) slog.Attr
		want    slog.Attr
	}{
		{name: "no callback", want: slog.String(slog.TimeKey, timestamp.UTC().Format(layout))},
		{name: "unchanged", replace: func(attr slog.Attr) slog.Attr { return attr }, want: slog.String(slog.TimeKey, timestamp.UTC().Format(layout))},
		{name: "different time", replace: func(attr slog.Attr) slog.Attr { return slog.Time(attr.Key, timestamp.Add(time.Hour)) }, want: slog.String(slog.TimeKey, timestamp.Add(time.Hour).UTC().Format(layout))},
		{name: "renamed", replace: func(attr slog.Attr) slog.Attr { attr.Key = "event_time"; return attr }, want: slog.Time("event_time", timestamp)},
		{name: "removed", replace: func(slog.Attr) slog.Attr { return slog.Attr{} }},
		{name: "string", replace: func(attr slog.Attr) slog.Attr { return slog.String(attr.Key, "custom timestamp") }, want: slog.String(slog.TimeKey, "custom timestamp")},
	} {
		for _, format := range []slogx.Format{slogx.JSON, slogx.Text} {
			t.Run(test.name+"/"+formatName(format), func(t *testing.T) {
				var output, nativeOutput bytes.Buffer
				var replace func([]string, slog.Attr) slog.Attr
				seenTime := false
				if test.replace != nil {
					replace = func(groups []string, attr slog.Attr) slog.Attr {
						if len(groups) == 0 && attr.Key == slog.TimeKey {
							if attr.Value.Kind() != slog.KindTime || !attr.Value.Time().Equal(timestamp) {
								t.Fatalf("callback time = %v, want native time.Time %v", attr.Value, timestamp)
							}
							seenTime = true
							return test.replace(attr)
						}
						return attr
					}
				}
				logger := slogx.New(slogx.Options{Format: format, Writer: &output, TimeLayout: layout, ReplaceAttr: replace})
				native := nativeTraceHandler(format, &nativeOutput, &slog.HandlerOptions{ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
					if len(groups) == 0 && attr.Key == slog.TimeKey {
						return test.want
					}
					return attr
				}})
				record := slog.NewRecord(timestamp, slog.LevelInfo, "request", 0)
				record.AddAttrs(slog.Group("nested", slog.Time(slog.TimeKey, timestamp)))
				for _, handler := range []slog.Handler{logger.Handler(), native} {
					if err := handler.Handle(t.Context(), record); err != nil {
						t.Fatal(err)
					}
				}
				if test.replace != nil && !seenTime {
					t.Fatal("callback did not receive the record timestamp")
				}
				if output.String() != nativeOutput.String() {
					t.Fatalf("output = %s, want native replacement %s", &output, &nativeOutput)
				}
			})
		}
	}
}

func TestReplaceAttrRedactsDecoratedFormats(t *testing.T) {
	for _, test := range []struct {
		name     string
		format   slogx.Format
		severity string
	}{
		{name: "colored text", format: slogx.TextColored, severity: "level=\x1b[31mERROR\x1b[0m"},
		{name: "systemd", format: slogx.Systemd, severity: "<3>"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slogx.New(slogx.Options{
				Format: test.format, Writer: &output,
				ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
					if attr.Key == "secret" {
						return slog.String(attr.Key, "[redacted]")
					}
					return attr
				},
			})
			record := slog.NewRecord(testTimestamp(0), slog.LevelError, "request failed", 0)
			record.AddAttrs(slog.String("secret", "sensitive"))
			if err := logger.Handler().Handle(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			if actual := output.String(); !strings.Contains(actual, test.severity) || !strings.Contains(actual, "secret=[redacted]") || strings.Contains(actual, "sensitive") {
				t.Fatalf("redaction or severity decoration was lost: %q", actual)
			}
		})
	}
}

func TestDecoratedFormatsReportWriterFailures(t *testing.T) {
	writeError := errors.New("write failed")
	for _, format := range []struct {
		name  string
		value slogx.Format
	}{
		{name: "colored text", value: slogx.TextColored},
		{name: "systemd", value: slogx.Systemd},
	} {
		for _, test := range []struct {
			name   string
			writer writerFunc
			want   error
		}{
			{
				name: "error",
				writer: func([]byte) (int, error) {
					return 0, writeError
				},
				want: writeError,
			},
			{
				name: "short write",
				writer: func(record []byte) (int, error) {
					return len(record) - 1, nil
				},
				want: io.ErrShortWrite,
			},
		} {
			t.Run(format.name+"/"+test.name, func(t *testing.T) {
				logger := slogx.New(slogx.Options{
					Format: format.value,
					Writer: test.writer,
				})
				record := slog.NewRecord(testTimestamp(0), slog.LevelInfo, "probe", 0)
				err := logger.Handler().Handle(context.Background(), record)
				if !errors.Is(err, test.want) {
					t.Fatalf("handle error = %v, want %v", err, test.want)
				}
			})
		}
	}
}

type writerFunc func([]byte) (int, error)

func (write writerFunc) Write(record []byte) (int, error) {
	return write(record)
}
