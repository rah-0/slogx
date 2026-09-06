package slogx

import "log/slog"

// Format controls the output format used by Slogx constructors.
type Format uint8

const (
	// Text selects slog.TextHandler.
	Text Format = iota
	// JSON selects slog.JSONHandler.
	JSON
	// TextColored selects slog.TextHandler with colored standard level names.
	TextColored
	// Systemd selects slog.TextHandler with systemd journal priority prefixes.
	Systemd
)

func formatTimestamp(timeLayout string, replaceAttr func([]string, slog.Attr) slog.Attr) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, attribute slog.Attr) slog.Attr {
		if replaceAttr != nil {
			attribute = replaceAttr(groups, attribute)
		}
		if len(groups) == 0 &&
			attribute.Key == slog.TimeKey &&
			attribute.Value.Kind() == slog.KindTime {
			attribute.Value = slog.StringValue(attribute.Value.Time().UTC().Format(timeLayout))
		}
		return attribute
	}
}
