package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	var output bytes.Buffer
	run(&output)
	var record map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for key, length := range map[string]int{"trace_id": 32, "span_id": 16} {
		var id string
		if err := json.Unmarshal(record[key], &id); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if len(id) != length || id != strings.ToLower(id) || id == strings.Repeat("0", length) {
			t.Fatalf("%s = %q, want a nonzero %d-character lowercase hex ID", key, id, length)
		}
		if _, err := hex.DecodeString(id); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	if _, exists := record["span"]; exists {
		t.Fatal("generating identifiers must not create a completed span")
	}
}
