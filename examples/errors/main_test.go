package main

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/rah-0/slogx"
)

func TestRunLogsThreeErrorLayersOnce(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	var output bytes.Buffer
	run(&output)

	decoder := slogx.NewJSONDecoder(&output)
	record, err := decoder.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if record.Level != slog.LevelError || record.Message != "request failed" {
		t.Fatalf("record = %+v, want request failed at ERROR", record)
	}
	if len(record.Attributes) != 1 || record.Attributes[0].Key != "err" {
		t.Fatalf("attributes = %v, want one structured err", record.Attributes)
	}
	want := `{"msg":"handle request","attrs":{"http":{"method":"GET","path":"/items/42"}},"cause":{"msg":"load item","attrs":{"cache_hit":false},"cause":{"msg":"query item","attrs":{"db":{"item_id":42,"attempt":2}},"cause":"connection refused"}}}`
	if got := string(record.Attributes[0].Value); got != want {
		t.Fatalf("error layers = %s, want %s", got, want)
	}
	if _, err := decoder.Decode(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected exactly one record, next decode returned %v", err)
	}
}

func TestWrappingPreservesCause(t *testing.T) {
	err := handleRequest(42)
	if !errors.Is(err, ErrConnectionRefused) {
		t.Fatalf("error chain lost original failure: %v", err)
	}
	if want := "handle request: load item: query item: connection refused"; err.Error() != want {
		t.Fatalf("error text = %q, want %q", err.Error(), want)
	}
}
