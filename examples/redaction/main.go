package main

import (
	"io"
	"log/slog"
	"os"
	"slices"

	"github.com/rah-0/slogx"
)

func main() {
	run(os.Stdout)
}

func run(output io.Writer) {
	logger := slogx.New(slogx.Options{
		Format: slogx.JSON,
		Writer: output,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case "password", "token":
				return slog.Attr{}
			case "email":
				if slices.Equal(groups, []string{"request", "auth"}) {
					return slog.String(attr.Key, "[REDACTED]")
				}
			}
			return attr
		},
	}).WithGroup("request").With(
		slog.Group("auth", "email", "customer@example.com", "password", "demo-password"),
	)

	logger.Info("request completed",
		slog.Group("http", "method", "GET", "token", "demo-token"),
		slog.Group("contact", "email", "support@example.com"),
	)
}
