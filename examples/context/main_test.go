package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
)

func TestRunAccumulatesAttributesWithoutChangingParentsOrSiblings(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	var output bytes.Buffer
	run(&output)

	http := map[string]any{"method": "GET", "path": "/items/42"}
	want := []map[string]any{
		{"level": "INFO", "msg": "request started", "request_id": "req-123", "http": http},
		{"level": "INFO", "msg": "item loading", "request_id": "req-123", "http": http, "item_id": float64(42), "cache_hit": false},
		{"level": "INFO", "msg": "query started", "request_id": "req-123", "http": http, "item_id": float64(42), "cache_hit": false, "db": map[string]any{"attempt": float64(2)}},
		{"level": "ERROR", "msg": "request failed", "request_id": "req-123", "http": http, "err": "context deadline exceeded"},
		{"level": "INFO", "msg": "sibling operation", "request_id": "req-123", "job": "warm cache"},
		{"level": "INFO", "msg": "original context", "request_id": "req-123"},
	}
	decoder := json.NewDecoder(&output)
	for i, expected := range want {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		delete(record, "time")
		if !reflect.DeepEqual(record, expected) {
			t.Fatalf("record %d = %v, want %v", i, record, expected)
		}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected extra record: %v, error %v", extra, err)
	}
}
