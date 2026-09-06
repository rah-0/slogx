package main

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/rah-0/slogx"
)

func main() {
	run(os.Stdout)
}

func run(output io.Writer) {
	slog.SetDefault(slogx.New(slogx.Options{Format: slogx.JSON, Writer: output}))
	// request_id is optional application metadata, not a required slogx field.
	ctx := slogx.WithAttrs(context.Background(), slog.String("request_id", "req-123"))
	handleRequest(ctx)

	sibling := slogx.WithAttrs(ctx, slog.String("job", "warm cache"))
	slog.InfoContext(sibling, "sibling operation")
	slog.InfoContext(ctx, "original context")
}

func handleRequest(ctx context.Context) {
	ctx = slogx.WithAttrs(ctx, slog.Group("http", "method", "GET", "path", "/items/42"))
	slog.InfoContext(ctx, "request started")
	if err := loadItem(ctx, 42); err != nil {
		// Child context attributes do not travel back up with the returned error.
		slogx.ErrorContext(ctx, "request failed", err)
	}
}

func loadItem(ctx context.Context, id int) error {
	ctx = slogx.WithAttrs(ctx, slog.Int("item_id", id), slog.Bool("cache_hit", false))
	slog.InfoContext(ctx, "item loading")
	return queryItem(ctx)
}

func queryItem(ctx context.Context) error {
	ctx = slogx.WithAttrs(ctx, slog.Group("db", "attempt", 2))
	slog.InfoContext(ctx, "query started")
	return context.DeadlineExceeded // Simulated query timeout.
}
