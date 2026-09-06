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
	logger := slogx.New(slogx.Options{Format: slogx.JSON, Writer: output})
	logger.Info("identifiers generated",
		slog.String("trace_id", slogx.NewTraceID()),
		slog.String("span_id", slogx.NewSpanID()),
	)
}
