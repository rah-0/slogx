package slogx_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rah-0/slogx"
)

const (
	w3cTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	w3cSpanID  = "00f067aa0ba902b7"
	w3cParent  = "00-" + w3cTraceID + "-" + w3cSpanID + "-01"
)

func TestTraceContextNilParent(t *testing.T) {
	sc := slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID, TraceFlags: 1}
	ctx := slogx.ContextWithSpanContext(nil, sc)
	if got := slogx.SpanContextFromContext(ctx); got != sc {
		t.Fatalf("span context = %+v, want %+v", got, sc)
	}
	ctx, span := slogx.StartSpan(nil, "root")
	if !span.SpanContext().IsValid() || slogx.SpanFromContext(ctx) != span {
		t.Fatal("nil context did not start a valid root span")
	}
	span.End()
	headers := make(http.Header)
	headers.Set("traceparent", w3cParent)
	sc.Remote = true
	if got := slogx.SpanContextFromContext(slogx.ExtractTraceContext(nil, headers)); got != sc {
		t.Fatalf("extracted span context = %+v, want %+v", got, sc)
	}
	if got := slogx.SpanContextFromContext(slogx.ExtractTraceContext(nil, nil)); got != (slogx.SpanContext{}) {
		t.Fatalf("empty headers span context = %+v, want zero value", got)
	}
}

func TestParseTraceparent(t *testing.T) {
	for _, test := range []struct {
		name, value string
		flags       byte
	}{
		{name: "sampled", value: w3cParent, flags: 1},
		{name: "not sampled", value: w3cParent[:53] + "00"},
		{name: "reserved flags ignored", value: w3cParent[:53] + "fe"},
		{name: "sampled among reserved flags", value: w3cParent[:53] + "ff", flags: 1},
		{name: "future version", value: "01" + w3cParent[2:], flags: 1},
		{name: "highest future version", value: "fe" + w3cParent[2:] + "-unknown-fields", flags: 1},
		{name: "future fields are opaque", value: "01" + w3cParent[2:] + "-UPPER_unknown", flags: 1},
		{name: "future separator only", value: "01" + w3cParent[2:] + "-", flags: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			sc, err := slogx.ParseTraceparent(test.value)
			if err != nil {
				t.Fatalf("parse traceparent: %v", err)
			}
			want := slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID, TraceFlags: test.flags, Remote: true}
			if sc != want || !sc.IsValid() || sc.IsSampled() != (test.flags == 1) {
				t.Fatalf("span context = %+v, want %+v", sc, want)
			}
			if actual, want := sc.Traceparent(), w3cParent[:53]+fmt.Sprintf("%02x", test.flags); actual != want {
				t.Fatalf("formatted traceparent = %q, want %q", actual, want)
			}
		})
	}
}

func TestParseTraceparentRejectsInvalidValues(t *testing.T) {
	for name, test := range map[string]struct {
		value string
		err   error
	}{
		"empty":                 {"", slogx.ErrInvalidTraceparentLength},
		"too short":             {w3cParent[:54], slogx.ErrInvalidTraceparentLength},
		"version 00 extension":  {w3cParent + "-extra", slogx.ErrInvalidTraceparentLength},
		"version ff":            {"ff" + w3cParent[2:], slogx.ErrInvalidTraceparentVersion},
		"uppercase version":     {"FE" + w3cParent[2:], slogx.ErrInvalidTraceparentVersion},
		"nonhex version":        {"0g" + w3cParent[2:], slogx.ErrInvalidTraceparentVersion},
		"version separator":     {w3cParent[:2] + "_" + w3cParent[3:], slogx.ErrInvalidTraceparentVersion},
		"trace separator":       {w3cParent[:35] + "_" + w3cParent[36:], slogx.ErrInvalidTraceparentTraceID},
		"span separator":        {w3cParent[:52] + "_" + w3cParent[53:], slogx.ErrInvalidTraceparentSpanID},
		"zero trace ID":         {"00-" + strings.Repeat("0", 32) + w3cParent[35:], slogx.ErrInvalidTraceparentTraceID},
		"zero span ID":          {w3cParent[:36] + strings.Repeat("0", 16) + w3cParent[52:], slogx.ErrInvalidTraceparentSpanID},
		"uppercase trace ID":    {"00-" + strings.ToUpper(w3cTraceID) + w3cParent[35:], slogx.ErrInvalidTraceparentTraceID},
		"uppercase span ID":     {w3cParent[:36] + strings.ToUpper(w3cSpanID) + w3cParent[52:], slogx.ErrInvalidTraceparentSpanID},
		"nonhex trace ID":       {w3cParent[:3] + "g" + w3cParent[4:], slogx.ErrInvalidTraceparentTraceID},
		"nonhex span ID":        {w3cParent[:36] + "g" + w3cParent[37:], slogx.ErrInvalidTraceparentSpanID},
		"nonhex flags":          {w3cParent[:53] + "0g", slogx.ErrInvalidTraceparentFlags},
		"uppercase flags":       {w3cParent[:53] + "0F", slogx.ErrInvalidTraceparentFlags},
		"leading whitespace":    {" " + w3cParent, slogx.ErrInvalidTraceparentVersion},
		"trailing whitespace":   {w3cParent + " ", slogx.ErrInvalidTraceparentLength},
		"comma joined parents":  {w3cParent + "," + w3cParent, slogx.ErrInvalidTraceparentLength},
		"future bad separator":  {"01" + w3cParent[2:] + "extra", slogx.ErrInvalidTraceparentLength},
		"future invalid prefix": {"01" + w3cParent[2:53] + "zz-extra", slogx.ErrInvalidTraceparentFlags},
	} {
		t.Run(name, func(t *testing.T) {
			sc, err := slogx.ParseTraceparent(test.value)
			if !errors.Is(err, test.err) || sc != (slogx.SpanContext{}) {
				t.Fatalf("parse %q = %+v, %v, want zero context and %v", test.value, sc, err, test.err)
			}
			if !errors.Is(fmt.Errorf("extract parent: %w", err), test.err) {
				t.Fatal("sentinel identity was lost through wrapping")
			}
		})
	}
}

func TestSpanContextValidityAndFormatting(t *testing.T) {
	valid := slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID, TraceFlags: 0xff}
	if !valid.IsValid() || !valid.IsSampled() || valid.Traceparent() != w3cParent {
		t.Fatalf("valid span context = %+v, traceparent %q", valid, valid.Traceparent())
	}
	for _, sc := range []slogx.SpanContext{
		{},
		{TraceID: w3cTraceID},
		{SpanID: w3cSpanID},
		{TraceID: strings.Repeat("0", 32), SpanID: w3cSpanID},
		{TraceID: w3cTraceID, SpanID: strings.Repeat("0", 16)},
		{TraceID: w3cTraceID[:31], SpanID: w3cSpanID},
		{TraceID: w3cTraceID, SpanID: strings.ToUpper(w3cSpanID)},
	} {
		if sc.IsValid() || sc.Traceparent() != "" {
			t.Fatalf("invalid span context = %+v, traceparent %q", sc, sc.Traceparent())
		}
	}
}

func TestSpanContextPreservesContextAndIsolation(t *testing.T) {
	type privateKey struct{}
	base, cancel := context.WithDeadline(context.WithValue(context.Background(), privateKey{}, "private"), time.Now().Add(time.Hour))
	t.Cleanup(cancel)
	sc := slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID, TraceFlags: 1}
	parent := slogx.ContextWithSpanContext(base, sc)
	childSC := sc
	childSC.SpanID = "123456789abcdef0"
	child := slogx.ContextWithSpanContext(parent, childSC)
	cleared := slogx.ContextWithSpanContext(parent, slogx.SpanContext{})
	sc.SpanID = "changed"
	if slogx.SpanContextFromContext(parent).SpanID != w3cSpanID || slogx.SpanContextFromContext(child) != childSC {
		t.Fatal("replacing a span context changed its parent or stored value")
	}
	if slogx.SpanContextFromContext(base).IsValid() || slogx.SpanContextFromContext(cleared).IsValid() || slogx.SpanContextFromContext(nil).IsValid() {
		t.Fatal("unexpected span context on base, cleared, or nil context")
	}
	gotDeadline, _ := child.Deadline()
	wantDeadline, _ := base.Deadline()
	if child.Value(privateKey{}) != "private" || !gotDeadline.Equal(wantDeadline) || child.Done() != base.Done() {
		t.Fatal("context values, deadline, or cancellation channel changed")
	}
	cancel()
	if !errors.Is(child.Err(), context.Canceled) {
		t.Fatalf("child error = %v, want cancellation", child.Err())
	}
}

func TestExtractAndInjectTraceContext(t *testing.T) {
	headers := make(http.Header)
	headers.Set("TRACEPARENT", " \t"+w3cParent+"\t ")
	headers.Add("tracestate", "vendor=opaque")
	headers.Add("tRaCeStAtE", " \tother=second value ")
	ctx := slogx.ExtractTraceContext(context.Background(), headers)
	sc := slogx.SpanContextFromContext(ctx)
	want := slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID, TraceFlags: 1, TraceState: "vendor=opaque, \tother=second value ", Remote: true}
	if sc != want {
		t.Fatalf("extracted context = %+v, want %+v", sc, want)
	}
	out := make(http.Header)
	out.Set("TRACEPARENT", "old")
	out.Set("tracestate", "old=state")
	out.Set("Accept", "application/json")
	slogx.InjectTraceContext(ctx, out)
	if out.Get("Traceparent") != w3cParent || out.Get("Tracestate") != want.TraceState || out.Get("Accept") != "application/json" || len(out) != 3 {
		t.Fatalf("injected headers = %v", out)
	}
	if headers.Get("Traceparent") != " \t"+w3cParent+"\t " {
		t.Fatal("extraction mutated source headers")
	}
	slogx.InjectTraceContext(ctx, nil)
	if actual := slogx.SpanContextFromContext(slogx.ExtractTraceContext(context.Background(), out)); actual != want {
		t.Fatalf("propagated context = %+v, want %+v", actual, want)
	}
}

func TestInvalidTraceparentClearsInheritedContext(t *testing.T) {
	base := slogx.ContextWithSpanContext(context.Background(), slogx.SpanContext{TraceID: w3cTraceID, SpanID: w3cSpanID})
	for name, headers := range map[string]http.Header{
		"absent":           nil,
		"empty slice":      {"Traceparent": nil},
		"empty":            {"Traceparent": {""}},
		"invalid":          {"Traceparent": {"invalid"}},
		"repeated":         {"Traceparent": {w3cParent, w3cParent}},
		"joined":           {"Traceparent": {w3cParent + "," + w3cParent}},
		"state only":       {"Tracestate": {"vendor=opaque"}},
		"newline trimming": {"Traceparent": {"\n" + w3cParent}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := slogx.ExtractTraceContext(base, headers)
			if actual := slogx.SpanContextFromContext(ctx); actual != (slogx.SpanContext{}) {
				t.Fatalf("extracted invalid headers = %+v, want zero context", actual)
			}
			out := http.Header{"Traceparent": {w3cParent}, "Tracestate": {"vendor=old"}}
			slogx.InjectTraceContext(ctx, out)
			if len(out) != 0 {
				t.Fatalf("invalid context retained old headers: %v", out)
			}
			if !slogx.SpanContextFromContext(base).IsValid() {
				t.Fatal("extraction changed the base context")
			}
		})
	}
}

func TestTraceStateValidation(t *testing.T) {
	var entries []string
	for i := range 32 {
		entries = append(entries, fmt.Sprintf("vendor%d=value", i))
	}
	for _, test := range []struct {
		name, state string
		valid       bool
	}{
		{name: "empty", valid: true},
		{name: "empty members", state: " ,\t,vendor=opaque,, ", valid: true},
		{name: "value leading space", state: "vendor= value", valid: true},
		{name: "opaque punctuation", state: "vendor=!#$%&'()*+-./:;<>?@[\\]^_`{|}~", valid: true},
		{name: "simple key length", state: strings.Repeat("a", 256) + "=v", valid: true},
		{name: "multitenant", state: "1tenant@system=value", valid: true},
		{name: "max multitenant key", state: strings.Repeat("a", 241) + "@" + strings.Repeat("b", 14) + "=v", valid: true},
		{name: "maximum value", state: "vendor=" + strings.Repeat("a", 256), valid: true},
		{name: "32 entries", state: strings.Join(entries, ","), valid: true},
		{name: "over 512 bytes allowed", state: "first=" + strings.Repeat("a", 256) + ",second=" + strings.Repeat("b", 256), valid: true},
		{name: "empty value", state: "vendor="},
		{name: "blank value", state: "vendor= \t"},
		{name: "empty key", state: "=value"},
		{name: "key uppercase", state: "Vendor=value"},
		{name: "simple leading digit", state: "1vendor=value"},
		{name: "leading underscore", state: "_vendor=value"},
		{name: "key too long", state: strings.Repeat("a", 257) + "=v"},
		{name: "tenant too long", state: strings.Repeat("a", 242) + "@b=v"},
		{name: "system too long", state: "a@" + strings.Repeat("b", 15) + "=v"},
		{name: "empty tenant", state: "@system=value"},
		{name: "empty system", state: "tenant@=value"},
		{name: "system leading digit", state: "tenant@1system=value"},
		{name: "multiple at signs", state: "a@b@c=value"},
		{name: "duplicate key", state: "vendor=first,vendor=second"},
		{name: "33 entries", state: strings.Join(entries, ",") + ",other=value"},
		{name: "too many empty members", state: strings.Repeat(",", 32)},
		{name: "value too long", state: "vendor=" + strings.Repeat("a", 257)},
		{name: "value equals", state: "vendor=a=b"},
		{name: "value tab", state: "vendor=a\tb"},
		{name: "value newline", state: "vendor=a\nb"},
		{name: "value nonascii", state: "vendor=é"},
		{name: "value del", state: "vendor=a\x7f"},
		{name: "key space before equals", state: "vendor =value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := http.Header{"Traceparent": {w3cParent}, "Tracestate": {test.state}}
			sc := slogx.SpanContextFromContext(slogx.ExtractTraceContext(context.Background(), headers))
			want := ""
			if test.valid {
				want = test.state
			}
			if !sc.IsValid() || sc.TraceState != want {
				t.Fatalf("extracted context = %+v, want valid parent and tracestate %q", sc, want)
			}
			// Direct construction must not bypass validation during injection.
			sc.TraceState = test.state
			out := http.Header{"Tracestate": {"old=value"}}
			slogx.InjectTraceContext(slogx.ContextWithSpanContext(context.Background(), sc), out)
			if strings.Trim(want, " \t,") == "" {
				want = ""
			}
			if out.Get("Traceparent") != w3cParent || out.Get("Tracestate") != want {
				t.Fatalf("injected headers = %v, want parent and state %q", out, want)
			}
		})
	}
}

func TestRepeatedTraceStateValidation(t *testing.T) {
	headers := make(http.Header)
	headers.Set("traceparent", w3cParent)
	headers.Add("Tracestate", "vendor=one")
	headers.Add("tracestate", "vendor=two")
	sc := slogx.SpanContextFromContext(slogx.ExtractTraceContext(context.Background(), headers))
	if !sc.IsValid() || sc.TraceState != "" {
		t.Fatalf("duplicate tracestate key = %+v, want valid parent without state", sc)
	}
}

func FuzzParseTraceparent(f *testing.F) {
	for _, seed := range []string{"", w3cParent, "01" + w3cParent[2:] + "-extra", strings.Repeat("0", 55)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		sc, err := slogx.ParseTraceparent(value)
		if err != nil {
			if sc != (slogx.SpanContext{}) {
				t.Fatal("invalid input returned a nonzero span context")
			}
			return
		}
		if !sc.IsValid() || len(sc.Traceparent()) != 55 {
			t.Fatalf("accepted invalid traceparent %q", value)
		}
		roundTrip, err := slogx.ParseTraceparent(sc.Traceparent())
		if err != nil || roundTrip != sc {
			t.Fatalf("round trip = %+v, %v, want %+v", roundTrip, err, sc)
		}
	})
}
