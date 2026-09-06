package slogx_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/rah-0/slogx"
)

func TestGeneratedTraceIdentifiers(t *testing.T) {
	for _, test := range []struct {
		name     string
		generate func() string
		size     int
	}{
		{name: "trace", generate: slogx.NewTraceID, size: 16},
		{name: "span", generate: slogx.NewSpanID, size: 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			seen := make(map[string]bool)
			for range 64 {
				id := test.generate()
				if len(id) != test.size*2 {
					t.Fatalf("ID length = %d, want %d", len(id), test.size*2)
				}
				decoded, err := hex.DecodeString(id)
				if err != nil || hex.EncodeToString(decoded) != id {
					t.Fatalf("ID %q is not lowercase hexadecimal", id)
				}
				if id == strings.Repeat("0", test.size*2) {
					t.Fatal("generated the forbidden all-zero ID")
				}
				if seen[id] {
					t.Fatalf("generated a repeated ID: %s", id)
				}
				seen[id] = true
			}
		})
	}
}

func TestTraceIdentifiersUseRandomBytesAndRejectAllZero(t *testing.T) {
	// These subtests replace a process-wide random source and must not run in parallel.
	for _, test := range []struct {
		name     string
		generate func() string
		size     int
	}{
		{name: "trace", generate: slogx.NewTraceID, size: 16},
		{name: "span", generate: slogx.NewSpanID, size: 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, candidate := range []struct {
				name     string
				position int
			}{
				{name: "trailing zeros", position: 0},
				{name: "leading zeros", position: test.size - 1},
			} {
				t.Run(candidate.name, func(t *testing.T) {
					valid := make([]byte, test.size)
					valid[candidate.position] = 0xab
					// Two invalid candidates followed by one valid ID with leading
					// or trailing zeros. Every random byte must survive encoding.
					input := append(make([]byte, test.size*2), valid...)
					reader := bytes.NewReader(input)
					previous := rand.Reader
					rand.Reader = reader
					t.Cleanup(func() { rand.Reader = previous })

					if id := test.generate(); id != hex.EncodeToString(valid) {
						t.Fatalf("ID = %q, want %q", id, hex.EncodeToString(valid))
					}
					if reader.Len() != 0 {
						t.Fatalf("%d bytes remain; zero candidates were not retried correctly", reader.Len())
					}
				})
			}
		})
	}
}
