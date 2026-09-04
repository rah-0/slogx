package slogx

import (
	"io"
	"log/slog"
	"os"
)

// Options configures a Logger created by New or NewDefault.
type Options struct {
	// AddSource includes the source code position of each log call when true.
	AddSource bool
	// Level reports the minimum enabled log level. If nil, LevelInfo is used.
	// Use a *slog.LevelVar to change the minimum level dynamically.
	Level Leveler
	// Format selects the output format. Its zero value is Text.
	Format Format
	// TimeLayout formats timestamps in UTC using Go's time layout syntax.
	// With New, an empty value preserves slog's native timestamp formatting.
	// NewDefault uses the preferred timestamp layout when it is empty.
	TimeLayout string
	// Writer receives log output.
	// If nil, New uses os.Stderr and NewDefault uses os.Stdout.
	Writer io.Writer
}

// New returns a native slog Logger configured by options.
func New(options Options) *Logger {
	writer := options.Writer
	if writer == nil {
		writer = os.Stderr
	}

	handlerOptions := &slog.HandlerOptions{
		AddSource: options.AddSource,
		Level:     options.Level,
	}
	if options.TimeLayout != "" {
		handlerOptions.ReplaceAttr = formatTimestamp(options.TimeLayout)
	}

	var handler slog.Handler
	switch options.Format {
	case Text:
		handler = slog.NewTextHandler(writer, handlerOptions)
	case JSON:
		handler = slog.NewJSONHandler(writer, handlerOptions)
	case TextColored:
		handler = slog.NewTextHandler(levelColorWriter{writer: writer}, handlerOptions)
	case Systemd:
		handler = slog.NewTextHandler(systemdLevelWriter{writer: writer}, handlerOptions)
	default:
		panic("slogx: unsupported format")
	}

	return slog.New(handler)
}

// NewDefault returns a Logger with Slogx output and timestamp defaults.
// Empty Writer and TimeLayout values use os.Stdout and the preferred timestamp layout.
func NewDefault(options Options) *Logger {
	if options.Writer == nil {
		options.Writer = os.Stdout
	}
	if options.TimeLayout == "" {
		options.TimeLayout = defaultTimeLayout
	}
	return New(options)
}

// SetDefault installs NewDefault(options) as slog's process-wide default Logger.
func SetDefault(options Options) {
	slog.SetDefault(NewDefault(options))
}
