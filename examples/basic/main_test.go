package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	var output bytes.Buffer
	run(&output)

	type Record struct {
		Time    time.Time    `json:"time"`
		Level   string       `json:"level"`
		Message string       `json:"msg"`
		Source  *slog.Source `json:"source"`
		Service string       `json:"service"`
		Version string       `json:"version"`
		Request struct {
			ID   string `json:"id"`
			HTTP struct {
				Method string `json:"method"`
				Status int    `json:"status"`
			} `json:"http"`
			ItemCount int `json:"item_count"`
		} `json:"request"`
	}
	var records []Record
	decoder := json.NewDecoder(&output)
	for {
		var record Record
		if err := decoder.Decode(&record); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	for _, record := range records {
		if record.Time.IsZero() || record.Level != "INFO" || record.Service != "catalog" {
			t.Fatalf("missing shared fields: %+v", record)
		}
		if record.Source == nil || filepath.Base(record.Source.File) != "main.go" ||
			!strings.HasSuffix(record.Source.Function, ".run") || record.Source.Line <= 0 {
			t.Fatalf("source = %+v, want the logging call in run", record.Source)
		}
	}
	if first := records[0]; first.Message != "service started" || first.Version != "1.0.0" || first.Request.ID != "" {
		t.Fatalf("startup record = %+v", first)
	}
	second := records[1]
	if second.Message != "request completed" || second.Request.ID != "req-42" ||
		second.Request.HTTP.Method != "GET" || second.Request.HTTP.Status != 200 || second.Request.ItemCount != 3 {
		t.Fatalf("request record = %+v", second)
	}
}
