package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	var output bytes.Buffer
	run(&output)
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want one per format", len(lines))
	}

	var record struct {
		Time    string `json:"time"`
		Level   string `json:"level"`
		Message string `json:"msg"`
		Format  string `json:"format"`
		Cache   struct {
			Name  string `json:"name"`
			Retry bool   `json:"retry"`
		} `json:"cache"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &record); err != nil {
		t.Fatal(err)
	}
	if record.Level != "WARN" || record.Message != "cache unavailable" ||
		record.Format != "json" || record.Cache.Name != "items" || !record.Cache.Retry {
		t.Fatalf("JSON output = %+v", record)
	}
	if _, err := time.Parse(timeLayout, record.Time); err != nil {
		t.Fatalf("JSON time does not use the configured layout: %v", err)
	}

	for _, example := range []struct {
		Index int
		Name  string
		Level string
	}{
		{0, "text", "level=WARN"},
		{2, "colored", "level=\x1b[33mWARN\x1b[0m"},
		{3, "systemd", "level=WARN"},
	} {
		line := lines[example.Index]
		for _, field := range []string{example.Level, "format=" + example.Name,
			`msg="cache unavailable"`, "cache.name=items", "cache.retry=true"} {
			if !strings.Contains(line, field) {
				t.Errorf("%s output missing %q: %q", example.Name, field, line)
			}
		}
		if example.Name == "systemd" {
			if !strings.HasPrefix(line, "<4>") {
				t.Fatalf("systemd WARN output lacks priority 4: %q", line)
			}
			line = strings.TrimPrefix(line, "<4>")
		}
		stamp, _, ok := strings.Cut(strings.TrimPrefix(line, `time="`), `"`)
		if _, err := time.Parse(timeLayout, stamp); !ok || err != nil {
			t.Errorf("%s timestamp = %q, error = %v", example.Name, stamp, err)
		}
		if example.Name != "colored" && strings.ContainsRune(line, '\x1b') {
			t.Errorf("%s output contains ANSI escapes", example.Name)
		}
	}
}
