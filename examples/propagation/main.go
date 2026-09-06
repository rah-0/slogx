package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/rah-0/slogx"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(output io.Writer) error {
	slog.SetDefault(slogx.New(slogx.Options{
		Format: slogx.JSON, AddSource: true, Writer: output,
	}))
	downstream := httptest.NewServer(http.HandlerFunc(serveInventory))
	defer downstream.Close()

	// Simulate a request carrying context from an upstream service.
	parent := slogx.SpanContext{
		TraceID: slogx.NewTraceID(), SpanID: slogx.NewSpanID(),
		TraceFlags: 1, TraceState: "example=opaque",
	}
	request := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	slogx.InjectTraceContext(slogx.ContextWithSpanContext(context.Background(), parent), request.Header)
	return handleRequest(request, downstream.Client(), downstream.URL)
}

func handleRequest(request *http.Request, client *http.Client, inventoryURL string) error {
	ctx := slogx.ExtractTraceContext(request.Context(), request.Header)
	ctx, span := slogx.StartSpan(ctx, "handle request")
	span.SetKind(slogx.SpanKindServer)
	defer span.End()
	slog.InfoContext(ctx, "request received",
		slog.String("traceparent", request.Header.Get("traceparent")),
		slog.String("tracestate", request.Header.Get("tracestate")),
	)

	if err := fetchInventory(ctx, client, inventoryURL); err != nil {
		err = slogx.Wrap(err, "handle request")
		span.RecordError(err)
		span.SetStatus(slogx.StatusError, "request failed")
		return err
	}
	span.SetStatus(slogx.StatusOK, "")
	return nil
}

func fetchInventory(ctx context.Context, client *http.Client, inventoryURL string) (err error) {
	ctx, span := slogx.StartSpan(ctx, "fetch inventory")
	span.SetKind(slogx.SpanKindClient)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(slogx.StatusError, "inventory request failed")
		}
		span.End()
	}()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, inventoryURL, nil)
	if err != nil {
		return slogx.Wrap(err, "create inventory request")
	}
	slogx.InjectTraceContext(ctx, request.Header)
	response, err := client.Do(request)
	if err != nil {
		return slogx.Wrap(err, "send inventory request")
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return slogx.Wrap(err, "read inventory response")
	}
	if response.StatusCode != http.StatusNoContent {
		return slogx.Wrap(ErrUnexpectedStatus, "fetch inventory", "status", response.StatusCode)
	}
	span.SetStatus(slogx.StatusOK, "")
	slog.InfoContext(ctx, "inventory response received")
	return nil
}

func serveInventory(writer http.ResponseWriter, request *http.Request) {
	ctx := slogx.ExtractTraceContext(request.Context(), request.Header)
	ctx, span := slogx.StartSpan(ctx, "serve inventory")
	span.SetKind(slogx.SpanKindServer)
	defer span.End()
	slog.InfoContext(ctx, "inventory request received",
		slog.String("traceparent", request.Header.Get("traceparent")),
		slog.String("tracestate", request.Header.Get("tracestate")),
	)
	span.SetStatus(slogx.StatusOK, "")
	writer.WriteHeader(http.StatusNoContent)
}
