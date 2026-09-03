package slogx

import (
	"io"
	"log/slog"
)

type systemdLevelWriter struct {
	writer io.Writer
}

func (w systemdLevelWriter) Write(record []byte) (int, error) {
	prefix := systemdInfo
	start, end := findTextLevel(record)
	if start != end {
		var level Level
		if level.UnmarshalText(record[start:end]) == nil {
			prefix = systemdLevelPrefix(level)
		}
	}

	prefixed := make([]byte, 0, len(prefix)+len(record))
	prefixed = append(prefixed, prefix...)
	prefixed = append(prefixed, record...)

	n, err := w.writer.Write(prefixed)
	if n == len(prefixed) {
		return len(record), err
	}
	if err != nil {
		return 0, err
	}
	return 0, io.ErrShortWrite
}

func systemdLevelPrefix(level Level) string {
	switch {
	case level >= slog.LevelError:
		return systemdError
	case level >= slog.LevelWarn:
		return systemdWarning
	case level >= slog.LevelInfo:
		return systemdInfo
	default:
		return systemdDebug
	}
}
