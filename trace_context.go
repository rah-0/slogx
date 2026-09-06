package slogx

import (
	"context"
	"net/http"
	"strings"
)

// Traceparent layout and tracestate limits from W3C Trace Context Level 1:
// https://www.w3.org/TR/2021/REC-trace-context-1-20211123/
const (
	traceIDHexLength         = 32
	spanIDHexLength          = 16
	traceparentVersionLength = 2
	traceparentFlagsLength   = 2

	traceparentTraceIDStart = traceparentVersionLength + 1
	traceparentTraceIDEnd   = traceparentTraceIDStart + traceIDHexLength
	traceparentSpanIDStart  = traceparentTraceIDEnd + 1
	traceparentSpanIDEnd    = traceparentSpanIDStart + spanIDHexLength
	traceparentFlagsStart   = traceparentSpanIDEnd + 1
	traceparentBaseLength   = traceparentFlagsStart + traceparentFlagsLength

	traceparentVersion00      = 0x00
	traceparentInvalidVersion = 0xff
	traceFlagSampled          = 0x01

	traceStateMaxMembers      = 32
	traceStateMaxKeyLength    = 256
	traceStateMaxValueLength  = 256
	traceStateMaxTenantLength = 241
	traceStateMaxSystemLength = 14
	traceStateValueMinByte    = ' '
	traceStateValueMaxByte    = '~'
)

const (
	traceHexDigitBits  = 4
	traceHexLetterBase = 10
)

// SpanContext identifies a span and carries W3C Trace Context propagation data.
// TraceID and SpanID use lowercase hexadecimal without separators. Remote is
// true for a parent extracted from another process. TraceState contains opaque
// vendor data, not application attributes.
type SpanContext struct {
	TraceID    string
	SpanID     string
	TraceFlags byte
	TraceState string
	Remote     bool
}

// IsValid reports whether both identifiers have the W3C format and are nonzero.
// TraceState validity is checked separately when propagating HTTP headers.
func (sc SpanContext) IsValid() bool {
	return validTraceIdentifier(sc.TraceID, traceIDHexLength) && validTraceIdentifier(sc.SpanID, spanIDHexLength)
}

// IsSampled reports whether the W3C sampled flag is set.
func (sc SpanContext) IsSampled() bool {
	return sc.TraceFlags&traceFlagSampled != 0
}

type spanContextKey struct{}

// ContextWithSpanContext returns a context carrying a copy of sc. The parent
// context and its cancellation, deadline, and other values are preserved.
// An invalid sc explicitly clears an inherited span context.
// A nil ctx uses [context.Background].
func ContextWithSpanContext(ctx context.Context, sc SpanContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, spanContextKey{}, sc)
}

// SpanContextFromContext returns the span context in ctx, or its zero value if
// none is present. A nil ctx also returns the zero value.
func SpanContextFromContext(ctx context.Context) SpanContext {
	if ctx == nil {
		return SpanContext{}
	}
	sc, _ := ctx.Value(spanContextKey{}).(SpanContext)
	return sc
}

// ParseTraceparent parses a W3C Trace Context Level 1 traceparent field value.
// The version, identifiers, and flags must use lowercase hexadecimal. Version 00
// has exactly 55 bytes. Versions 01 through fe accept additional bytes after a
// dash; that suffix is not parsed. Unknown flag bits are discarded, and version
// ff is invalid. The function does not trim whitespace.
// The returned span context is remote and has no TraceState.
// Invalid input returns [ErrInvalidTraceparentLength], [ErrInvalidTraceparentVersion],
// [ErrInvalidTraceparentTraceID], [ErrInvalidTraceparentSpanID], or [ErrInvalidTraceparentFlags].
func ParseTraceparent(value string) (SpanContext, error) {
	if len(value) < traceparentBaseLength {
		return SpanContext{}, ErrInvalidTraceparentLength
	}
	version, ok := traceHexByte(value[:traceparentVersionLength])
	if !ok || version == traceparentInvalidVersion || value[traceparentVersionLength] != '-' {
		return SpanContext{}, ErrInvalidTraceparentVersion
	}
	if (version == traceparentVersion00 && len(value) != traceparentBaseLength) ||
		(len(value) > traceparentBaseLength && value[traceparentBaseLength] != '-') {
		return SpanContext{}, ErrInvalidTraceparentLength
	}
	if value[traceparentTraceIDEnd] != '-' ||
		!validTraceIdentifier(value[traceparentTraceIDStart:traceparentTraceIDEnd], traceIDHexLength) {
		return SpanContext{}, ErrInvalidTraceparentTraceID
	}
	if value[traceparentSpanIDEnd] != '-' ||
		!validTraceIdentifier(value[traceparentSpanIDStart:traceparentSpanIDEnd], spanIDHexLength) {
		return SpanContext{}, ErrInvalidTraceparentSpanID
	}
	flags, ok := traceHexByte(value[traceparentFlagsStart:traceparentBaseLength])
	if !ok {
		return SpanContext{}, ErrInvalidTraceparentFlags
	}
	return SpanContext{
		TraceID:    value[traceparentTraceIDStart:traceparentTraceIDEnd],
		SpanID:     value[traceparentSpanIDStart:traceparentSpanIDEnd],
		TraceFlags: flags & traceFlagSampled,
		Remote:     true,
	}, nil
}

// Traceparent formats sc using W3C Trace Context version 00, emitting only the
// supported sampled flag. It returns an empty string if either ID is invalid.
func (sc SpanContext) Traceparent() string {
	if !sc.IsValid() {
		return ""
	}
	flags := "00"
	if sc.IsSampled() {
		flags = "01"
	}
	return "00-" + sc.TraceID + "-" + sc.SpanID + "-" + flags
}

// ExtractTraceContext returns ctx with the remote parent carried by headers.
// A missing, invalid, or repeated traceparent clears any inherited span context;
// starting a new span then starts a new trace. Invalid tracestate is discarded
// without losing a valid parent. Repeated tracestate fields are combined in
// order, as represented by the header's value slice.
//
// Use [http.Header.Set] or [http.Header.Add] to construct headers. Direct map
// keys must use [http.CanonicalHeaderKey], following net/http's convention.
func ExtractTraceContext(ctx context.Context, headers http.Header) context.Context {
	parents := headers.Values("traceparent")
	if len(parents) != 1 {
		return ContextWithSpanContext(ctx, SpanContext{})
	}
	sc, err := ParseTraceparent(strings.Trim(parents[0], " \t"))
	if err != nil {
		return ContextWithSpanContext(ctx, SpanContext{})
	}
	if state := strings.Join(headers.Values("tracestate"), ","); validTraceState(state) {
		sc.TraceState = state
	}
	return ContextWithSpanContext(ctx, sc)
}

// InjectTraceContext replaces headers' traceparent and tracestate with the
// current span context. An invalid span context removes both headers; invalid
// or empty tracestate is omitted. A nil header map is left unchanged.
// Headers follow the same canonical-key convention as [ExtractTraceContext].
func InjectTraceContext(ctx context.Context, headers http.Header) {
	headers.Del("traceparent")
	headers.Del("tracestate")
	sc := SpanContextFromContext(ctx)
	parent := sc.Traceparent()
	if headers == nil || parent == "" {
		return
	}
	headers.Set("traceparent", parent)
	if strings.Trim(sc.TraceState, " \t,") != "" && validTraceState(sc.TraceState) {
		headers.Set("tracestate", sc.TraceState)
	}
}

func validTraceIdentifier(value string, size int) bool {
	if len(value) != size {
		return false
	}
	nonzero := false
	for i := range len(value) {
		if _, ok := traceHexDigit(value[i]); !ok {
			return false
		}
		nonzero = nonzero || value[i] != '0'
	}
	return nonzero
}

func traceHexByte(value string) (byte, bool) {
	high, highOK := traceHexDigit(value[0])
	low, lowOK := traceHexDigit(value[1])
	return high<<traceHexDigitBits | low, highOK && lowOK
}

func traceHexDigit(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + traceHexLetterBase, true
	default:
		return 0, false
	}
}

func validTraceState(state string) bool {
	seen := make(map[string]bool)
	members := 0
	for member := range strings.SplitSeq(state, ",") {
		members++
		if members > traceStateMaxMembers {
			return false
		}
		member = strings.Trim(member, " \t")
		if member == "" {
			continue
		}
		key, value, ok := strings.Cut(member, "=")
		if !ok || !validTraceStateKey(key) || seen[key] || len(value) == 0 || len(value) > traceStateMaxValueLength {
			return false
		}
		for i := range len(value) {
			if value[i] < traceStateValueMinByte || value[i] > traceStateValueMaxByte || value[i] == ',' || value[i] == '=' {
				return false
			}
		}
		seen[key] = true
	}
	return true
}

func validTraceStateKey(key string) bool {
	if len(key) == 0 || len(key) > traceStateMaxKeyLength {
		return false
	}
	tenant, system, multi := strings.Cut(key, "@")
	if multi {
		if len(tenant) == 0 || len(tenant) > traceStateMaxTenantLength ||
			len(system) == 0 || len(system) > traceStateMaxSystemLength {
			return false
		}
		if !traceStateLower(system[0]) || (!traceStateLower(tenant[0]) && (tenant[0] < '0' || tenant[0] > '9')) {
			return false
		}
	} else if !traceStateLower(key[0]) {
		return false
	}
	for _, part := range []string{tenant, system} {
		for i := range len(part) {
			c := part[i]
			if !traceStateLower(c) && (c < '0' || c > '9') && c != '_' && c != '-' && c != '*' && c != '/' {
				return false
			}
		}
	}
	return true
}

func traceStateLower(value byte) bool {
	return value >= 'a' && value <= 'z'
}
