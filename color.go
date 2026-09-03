package slogx

import "io"

type levelColorWriter struct {
	writer io.Writer
}

func (w levelColorWriter) Write(record []byte) (int, error) {
	start, end := findTextLevel(record)
	color := levelColor(string(record[start:end]))
	if color == "" {
		return w.writer.Write(record)
	}

	colored := make([]byte, 0, len(record)+len(color)+len(ansiReset))
	colored = append(colored, record[:start]...)
	colored = append(colored, color...)
	colored = append(colored, record[start:end]...)
	colored = append(colored, ansiReset...)
	colored = append(colored, record[end:]...)

	n, err := w.writer.Write(colored)
	if n == len(colored) {
		return len(record), err
	}
	if err != nil {
		return 0, err
	}
	return 0, io.ErrShortWrite
}

func findTextLevel(record []byte) (start, end int) {
	for fieldStart := 0; fieldStart < len(record); {
		fieldEnd := textFieldEnd(record, fieldStart)
		field := record[fieldStart:fieldEnd]
		const prefix = "level="
		if len(field) >= len(prefix) && string(field[:len(prefix)]) == prefix {
			start = fieldStart + len(prefix)
			end = fieldEnd
			return start, end
		}

		fieldStart = fieldEnd + 1
	}
	return 0, 0
}

func textFieldEnd(record []byte, start int) int {
	quoted := false
	escaped := false
	for i := start; i < len(record); i++ {
		switch {
		case escaped:
			escaped = false
		case quoted && record[i] == '\\':
			escaped = true
		case record[i] == '"':
			quoted = !quoted
		case !quoted && (record[i] == ' ' || record[i] == '\n'):
			return i
		}
	}
	return len(record)
}

func levelColor(level string) string {
	switch level {
	case "DEBUG":
		return ansiGray
	case "INFO":
		return ansiGreen
	case "WARN":
		return ansiYellow
	case "ERROR":
		return ansiRed
	default:
		return ""
	}
}
