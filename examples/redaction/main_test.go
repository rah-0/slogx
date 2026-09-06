package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	var output bytes.Buffer
	run(&output)
	for _, secret := range []string{"customer@example.com", "demo-password", "demo-token"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("output contains %q", secret)
		}
	}
	var record struct {
		Message string `json:"msg"`
		Request struct {
			Auth struct {
				Email    string          `json:"email"`
				Password json.RawMessage `json:"password"`
			} `json:"auth"`
			HTTP struct {
				Method string          `json:"method"`
				Token  json.RawMessage `json:"token"`
			} `json:"http"`
			Contact struct {
				Email string `json:"email"`
			} `json:"contact"`
		} `json:"request"`
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Message != "request completed" || record.Request.Auth.Email != "[REDACTED]" ||
		record.Request.HTTP.Method != "GET" || record.Request.Contact.Email != "support@example.com" {
		t.Fatalf("grouped fields = %+v", record)
	}
	if len(record.Request.Auth.Password) != 0 || len(record.Request.HTTP.Token) != 0 {
		t.Fatal("password and token keys should be omitted")
	}
}
