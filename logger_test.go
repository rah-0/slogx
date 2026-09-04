package slogx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"runtime"
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
