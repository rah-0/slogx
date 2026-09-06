package slogx

import (
	"crypto/rand"
	"encoding/hex"
)

// NewTraceID returns a cryptographically random W3C Trace Context trace ID:
// 16 bytes encoded as 32 lowercase hexadecimal characters, never all zero.
// It generates an identifier only; it does not start a trace.
func NewTraceID() string {
	return newRandomID(16)
}

// NewSpanID returns a cryptographically random W3C Trace Context span ID:
// 8 bytes encoded as 16 lowercase hexadecimal characters, never all zero.
// This is the identifier format used by traceparent's parent-id field.
// It generates an identifier only; it does not start a span.
func NewSpanID() string {
	return newRandomID(8)
}

func newRandomID(size int) string {
	id := make([]byte, size)
	for {
		// Since Go 1.24, rand.Read fills id or terminates the process on a read error.
		rand.Read(id)
		for _, value := range id {
			if value != 0 {
				return hex.EncodeToString(id)
			}
		}
	}
}
