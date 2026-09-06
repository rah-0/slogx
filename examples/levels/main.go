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
	var level slog.LevelVar
	level.Set(slog.LevelInfo)
	logger := slogx.New(slogx.Options{
		Format: slogx.JSON,
		Level:  &level,
		Writer: output,
	})

	logger.Debug("hidden before change")
	logger.Info("service started")

	level.Set(slog.LevelDebug)
	logger.Debug("debug logging enabled", "attempt", 1)

	level.Set(slog.LevelWarn)
	logger.Info("hidden after change")
	logger.Warn("cache unavailable")
}
