package main

import (
	"io"
	"log/slog"
	"os"

	"github.com/rah-0/slogx"
)

func main() {
	run(os.Stdout)
}

func run(output io.Writer) {
	logger := slogx.New(slogx.Options{
		Format:    slogx.JSON,
		AddSource: true,
		Writer:    output,
	}).With("service", "catalog")

	logger.Info("service started", "version", "1.0.0")

	requestLogger := logger.WithGroup("request").With("id", "req-42")
	requestLogger.Info("request completed",
		slog.Group("http", "method", "GET", "status", 200),
		"item_count", 3,
	)
}
