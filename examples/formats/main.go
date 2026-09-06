package main

import (
	"io"
	"log/slog"
	"os"

	"github.com/rah-0/slogx"
)

const timeLayout = "2006/01/02 15:04:05"

func main() {
	run(os.Stdout)
}

func run(output io.Writer) {
	for _, example := range []struct {
		Name   string
		Format slogx.Format
	}{
		{"text", slogx.Text},
		{"json", slogx.JSON},
		{"colored", slogx.TextColored},
		{"systemd", slogx.Systemd},
	} {
		logger := slogx.New(slogx.Options{
			Format:     example.Format,
			TimeLayout: timeLayout,
			Writer:     output,
		})
		logger.Warn("cache unavailable",
			"format", example.Name,
			slog.Group("cache", "name", "items", "retry", true),
		)
	}
}
