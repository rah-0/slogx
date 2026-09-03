package slogx

import "log/slog"

func formatTimestamp(timeLayout string) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, attribute slog.Attr) slog.Attr {
		if len(groups) == 0 &&
			attribute.Key == slog.TimeKey &&
			attribute.Value.Kind() == slog.KindTime {
			attribute.Value = slog.StringValue(attribute.Value.Time().UTC().Format(timeLayout))
		}
		return attribute
	}
}
