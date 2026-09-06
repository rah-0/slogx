package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rah-0/slogx"
)

type LogRecord struct {
	Message     string      `json:"msg"`
	TraceID     string      `json:"trace_id"`
	SpanID      string      `json:"span_id"`
	Traceparent string      `json:"traceparent"`
	Tracestate  string      `json:"tracestate"`
	Span        *SpanRecord `json:"span"`
}

type SpanRecord struct {
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id"`
	TraceState   string `json:"trace_state"`
	TraceFlags   int    `json:"trace_flags"`
	Name         string `json:"name"`
	Kind         int    `json:"kind"`
	Status       int    `json:"status"`
}

func TestRun(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	var output bytes.Buffer
	if err := run(&output); err != nil {
		t.Fatal(err)
	}
	spans := make(map[string]LogRecord)
	logs := make(map[string]LogRecord)
	decoder := json.NewDecoder(&output)
	for {
		var record LogRecord
		if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if record.Span != nil {
			if _, exists := spans[record.Span.Name]; exists {
				t.Fatalf("duplicate span %q", record.Span.Name)
			}
			spans[record.Span.Name] = record
		} else {
			if _, exists := logs[record.Message]; exists {
				t.Fatalf("duplicate log %q", record.Message)
			}
			logs[record.Message] = record
		}
	}
	if len(spans) != 3 || len(logs) != 3 {
		t.Fatalf("got %d spans and %d logs, want 3 of each", len(spans), len(logs))
	}
	upstream, err := slogx.ParseTraceparent(logs["request received"].Traceparent)
	if err != nil {
		t.Fatal(err)
	}
	parentID := upstream.SpanID
	seen := map[string]bool{parentID: true}
	for index, name := range []string{"handle request", "fetch inventory", "serve inventory"} {
		record, exists := spans[name]
		if !exists {
			t.Fatalf("missing span %q", name)
		}
		span := record.Span
		if span.TraceID != upstream.TraceID || span.ParentSpanID != parentID || len(span.SpanID) != 16 || seen[span.SpanID] {
			t.Fatalf("broken distributed parent chain: %+v", span)
		}
		seen[span.SpanID] = true
		parentID = span.SpanID
		if span.TraceFlags != 1 || span.TraceState != "example=opaque" || span.Status != 1 {
			t.Fatalf("span lost propagation state or success status: %+v", span)
		}
		if span.Kind != []int{2, 3, 2}[index] || record.TraceID != span.TraceID || record.SpanID != span.SpanID {
			t.Fatalf("span lost kind or log correlation: %+v", record)
		}
	}
	forwarded, err := slogx.ParseTraceparent(logs["inventory request received"].Traceparent)
	if err != nil {
		t.Fatal(err)
	}
	if forwarded.TraceID != upstream.TraceID || forwarded.SpanID != spans["fetch inventory"].Span.SpanID || !forwarded.IsSampled() {
		t.Fatalf("outgoing HTTP request has wrong parent: %+v", forwarded)
	}
	for message, name := range map[string]string{
		"request received": "handle request", "inventory response received": "fetch inventory",
		"inventory request received": "serve inventory",
	} {
		record := logs[message]
		if record.TraceID != upstream.TraceID || record.SpanID != spans[name].Span.SpanID {
			t.Fatalf("log %q lost correlation", message)
		}
		if message != "inventory response received" && record.Tracestate != "example=opaque" {
			t.Fatalf("HTTP tracestate was not forwarded: %+v", record)
		}
	}
}

func TestRequestFailures(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	t.Cleanup(func() { slog.SetDefault(previous) })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	request := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	if err := handleRequest(request, server.Client(), server.URL); !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatalf("unexpected status error = %v", err)
	}
	if err := fetchInventory(context.Background(), server.Client(), ":bad URL"); err == nil {
		t.Fatal("invalid request URL was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fetchInventory(ctx, server.Client(), server.URL); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request error = %v", err)
	}
}

type ResponseTransport struct {
	Body io.ReadCloser
}

func (transport ResponseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusNoContent, Body: transport.Body}, nil
}

type FailingBody struct {
	Failure error
	Closed  bool
}

func (body *FailingBody) Read([]byte) (int, error) { return 0, body.Failure }
func (body *FailingBody) Close() error {
	body.Closed = true
	return nil
}

func TestResponseBodyFailureStillCloses(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	t.Cleanup(func() { slog.SetDefault(previous) })
	want := errors.New("response interrupted")
	body := &FailingBody{Failure: want}
	client := &http.Client{Transport: ResponseTransport{Body: body}}
	err := fetchInventory(context.Background(), client, "http://inventory.example/")
	if !errors.Is(err, want) || !body.Closed {
		t.Fatalf("response read error = %v, closed = %v", err, body.Closed)
	}
}
