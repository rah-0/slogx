package main

import (
	"fmt"
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
	if err := handleRequest(42); err != nil {
		// Log once where the operation is handled, retaining all three layers.
		slogx.Error("request failed", err)
	}
}

func handleRequest(id int) error {
	if err := loadItem(id); err != nil {
		return slogx.Wrap(err, "handle request",
			slog.Group("http", "method", "GET", "path", fmt.Sprintf("/items/%d", id)))
	}
	return nil
}

func loadItem(id int) error {
	if err := queryItem(id); err != nil {
		return slogx.Wrap(err, "load item", "cache_hit", false)
	}
	return nil
}

func queryItem(id int) error {
	return slogx.Wrap(ErrConnectionRefused, "query item",
		slog.Group("db", "item_id", id, "attempt", 2))
}
