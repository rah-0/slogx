package slogx_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/rah-0/slogx"
)

func TestSystemdPrefixesSlogLevels(t *testing.T) {
	for _, test := range []struct {
		level  slog.Level
		prefix string
	}{
		{level: slog.LevelDebug, prefix: "<7>"},
		{level: slog.LevelInfo, prefix: "<6>"},
		{level: slog.LevelWarn, prefix: "<4>"},
		{level: slog.LevelError, prefix: "<3>"},
		{level: slog.LevelInfo - 1, prefix: "<7>"},
		{level: slog.LevelInfo + 1, prefix: "<6>"},
		{level: slog.LevelWarn + 1, prefix: "<4>"},
		{level: slog.LevelError + 1, prefix: "<3>"},
	} {
		t.Run(test.level.String(), func(t *testing.T) {
			var output bytes.Buffer
			logger := slogx.New(slogx.Options{
				Level:      slog.LevelDebug - 1,
				Format:     slogx.Systemd,
				TimeLayout: "2006-01-02 15:04:05.000000",
				Writer:     &output,
			})
			record := slog.NewRecord(testTimestamp(123456789), test.level, "probe with spaces", 0)
			if err := logger.Handler().Handle(context.Background(), record); err != nil {
				t.Fatalf("handle record: %v", err)
			}

			actual := output.String()
			t.Logf("slogx output: %s", strings.TrimSpace(actual))
			if !strings.HasPrefix(actual, test.prefix) {
				t.Fatalf("output = %q, want prefix %q", actual, test.prefix)
			}
			if !strings.Contains(actual, `msg="probe with spaces"`) {
				t.Fatalf("output = %q, want quoted message", actual)
			}
			if strings.Contains(actual, "\x1b[") {
				t.Fatalf("output contains ANSI color: %q", actual)
			}
		})
	}
}
