package slogx_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/rah-0/slogx"
)

func TestJSONDecoderDecodesSlogxRecord(t *testing.T) {
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{
		Level:      slog.LevelDebug,
		Format:     slogx.JSON,
		TimeLayout: "2006-01-02 15:04:05.000000",
		Writer:     &output,
	})
	record := slog.NewRecord(testTimestamp(123456789), slog.LevelWarn+1, "probe with spaces", 0)
	record.AddAttrs(
		slog.Int64("request_id", 9_007_199_254_740_993),
		slog.Group("request", slog.String("method", "GET"), slog.Int("attempt", 2)),
		slog.String(slog.TimeKey, "application time"),
		slog.String(slog.LevelKey, "application level"),
		slog.String(slog.MessageKey, "attribute message"),
		slog.String(slog.SourceKey, "application source"),
		slog.Int("attempt", 1),
		slog.Int("attempt", 2),
	)
	if err := logger.Handler().Handle(context.Background(), record); err != nil {
		t.Fatalf("handle record: %v", err)
	}
	t.Logf("slogx output: %s", strings.TrimSpace(output.String()))

	decoder := slogx.NewJSONDecoder(&output)
	actual, err := decoder.Decode()
	if err != nil {
		t.Fatalf("decode JSON record: %v", err)
	}
	if actual.Time != "2026-09-03 20:45:12.123456" {
		t.Fatalf("time = %q", actual.Time)
	}
	if actual.Level != slog.LevelWarn+1 {
		t.Fatalf("level = %s, want %s", actual.Level, slog.LevelWarn+1)
	}
	if actual.Message != "probe with spaces" {
		t.Fatalf("message = %q", actual.Message)
	}
	if actual.Source != nil {
		t.Fatalf("source = %+v, want nil", actual.Source)
	}
	keys := make([]string, len(actual.Attributes))
	for index, attribute := range actual.Attributes {
		keys[index] = attribute.Key
	}
	expectedKeys := []string{
		"request_id",
		"request",
		slog.TimeKey,
		slog.LevelKey,
		slog.MessageKey,
		slog.SourceKey,
		"attempt",
		"attempt",
	}
	if !slices.Equal(keys, expectedKeys) {
		t.Fatalf("attribute keys = %q, want %q", keys, expectedKeys)
	}
	if actual := string(attributeValue(t, actual.Attributes, "request_id", 0)); actual != "9007199254740993" {
		t.Fatalf("request_id = %s", actual)
	}
	if actual := string(attributeValue(t, actual.Attributes, "request", 0)); actual != `{"method":"GET","attempt":2}` {
		t.Fatalf("request = %s", actual)
	}
	if actual := string(attributeValue(t, actual.Attributes, slog.LevelKey, 0)); actual != `"application level"` {
		t.Fatalf("level attribute = %s", actual)
	}
	if actual := string(attributeValue(t, actual.Attributes, slog.MessageKey, 0)); actual != `"attribute message"` {
		t.Fatalf("message attribute = %s", actual)
	}
	if first, second := string(attributeValue(t, actual.Attributes, "attempt", 0)), string(attributeValue(t, actual.Attributes, "attempt", 1)); first != "1" || second != "2" {
		t.Fatalf("attempt attributes = %s, %s", first, second)
	}

	if _, err := decoder.Decode(); err != io.EOF {
		t.Fatalf("second decode error = %v, want io.EOF", err)
	}
}

func TestJSONDecoderReadsMultipleRecords(t *testing.T) {
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{
		Format: slogx.JSON,
		Writer: &output,
	})
	logger.Info("first")
	logger.Warn("second")
	t.Logf("slogx output:\n%s", strings.TrimSpace(output.String()))

	decoder := slogx.NewJSONDecoder(&output)
	for _, expected := range []string{"first", "second"} {
		record, err := decoder.Decode()
		if err != nil {
			t.Fatalf("decode %q: %v", expected, err)
		}
		if record.Message != expected {
			t.Fatalf("message = %q, want %q", record.Message, expected)
		}
	}
	if _, err := decoder.Decode(); err != io.EOF {
		t.Fatalf("final decode error = %v, want io.EOF", err)
	}
}

func TestJSONDecoderReportsTruncatedFinalRecord(t *testing.T) {
	input := strings.NewReader("{\"time\":\"2026-09-03\",\"level\":\"INFO\",\"msg\":\"complete\"}\n{\"time\":\"2026-09-03\"")
	decoder := slogx.NewJSONDecoder(input)

	if _, err := decoder.Decode(); err != nil {
		t.Fatalf("decode complete record: %v", err)
	}
	if _, err := decoder.Decode(); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("truncated record error = %v, want non-EOF error", err)
	}
}

func TestJSONDecoderReportsMalformedRecord(t *testing.T) {
	decoder := slogx.NewJSONDecoder(strings.NewReader("not JSON\n"))
	if _, err := decoder.Decode(); err == nil {
		t.Fatal("decode malformed record returned nil error")
	}
}

func TestJSONDecoderRejectsNonObject(t *testing.T) {
	decoder := slogx.NewJSONDecoder(strings.NewReader("null\n"))
	if _, err := decoder.Decode(); err == nil {
		t.Fatal("decode non-object record returned nil error")
	}
}

func attributeValue(t *testing.T, attributes []slogx.JSONAttr, key string, occurrence int) []byte {
	t.Helper()

	for _, attribute := range attributes {
		if attribute.Key != key {
			continue
		}
		if occurrence == 0 {
			return attribute.Value
		}
		occurrence--
	}
	t.Fatalf("attribute %q occurrence not found", key)
	return nil
}
