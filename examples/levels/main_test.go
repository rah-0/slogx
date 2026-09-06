package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestRun(t *testing.T) {
	var output bytes.Buffer
	run(&output)
	decoder := json.NewDecoder(&output)
	for _, expected := range []struct {
		Level   string
		Message string
		Attempt int
	}{
		{"INFO", "service started", 0},
		{"DEBUG", "debug logging enabled", 1},
		{"WARN", "cache unavailable", 0},
	} {
		var record struct {
			Level   string `json:"level"`
			Message string `json:"msg"`
			Attempt int    `json:"attempt"`
		}
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record.Level != expected.Level || record.Message != expected.Message || record.Attempt != expected.Attempt {
			t.Fatalf("record = %+v, want %+v", record, expected)
		}
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("unexpected extra record: %s, error = %v", extra, err)
	}
}
