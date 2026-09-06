package slogx

import (
	"io"
	"log/slog"
	"os"
)

const defaultTimeLayout = "2006-01-02 15:04:05.000000"

// Options configures a Logger created by New or NewDefault.
type Options struct {
	// AddSource includes the source code position of each log call when true.
	// When this logger is installed as slog's default, [StartSpan] also records
	// its caller's file, line, and function on recording spans.
	AddSource bool
	// Level reports the minimum enabled log level. If nil, LevelInfo is used.
	// Use a *slog.LevelVar to change the minimum level dynamically.
	Level Leveler
	// Format selects the output format. Its zero value and unsupported values use Text.
	Format Format
	// TimeLayout formats timestamps in UTC using Go's time layout syntax.
	// With New, an empty value preserves slog's native timestamp formatting.
	// NewDefault uses the preferred timestamp layout when it is empty.
	TimeLayout string
	// ReplaceAttr rewrites resolved attributes with their native slog group path,
	// following slog.HandlerOptions.ReplaceAttr. A zero Attr removes the attribute.
	// When TimeLayout is set, ReplaceAttr runs first. Timestamp formatting then
	// applies only to returned root attributes named "time" with a time.Time value.
	// TextColored and Systemd derive decoration and priority from the rendered
	// level field; retain that field and its native value to preserve severity.
	ReplaceAttr func([]string, slog.Attr) slog.Attr
	// Writer receives log output.
	// If nil, New uses os.Stderr and NewDefault uses os.Stdout.
	Writer io.Writer
}

// New returns a native slog Logger configured by options.
// Context-aware logging methods include attributes attached with [WithAttrs].
// A valid [SpanContext] also adds trace_id and span_id. All added attributes
// follow the logger's active WithGroup, using native slog grouping and formatting.
func New(options Options) *Logger {
	writer := options.Writer
	if writer == nil {
		writer = os.Stderr
	}

	handlerOptions := &slog.HandlerOptions{
		AddSource:   options.AddSource,
		Level:       options.Level,
		ReplaceAttr: options.ReplaceAttr,
	}
	if options.TimeLayout != "" {
		handlerOptions.ReplaceAttr = formatTimestamp(options.TimeLayout, options.ReplaceAttr)
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
		handler = slog.NewTextHandler(writer, handlerOptions)
	}

	return slog.New(contextHandler{handler: handler})
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
