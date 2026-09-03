package slogx_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/rah-0/slogx"
)

func TestTextColoredColorsStandardLevels(t *testing.T) {
	for _, test := range []struct {
		level slog.Level
		color string
	}{
		{level: slog.LevelDebug, color: "\x1b[90m"},
		{level: slog.LevelInfo, color: "\x1b[32m"},
		{level: slog.LevelWarn, color: "\x1b[33m"},
		{level: slog.LevelError, color: "\x1b[31m"},
	} {
		t.Run(test.level.String(), func(t *testing.T) {
			var output bytes.Buffer
			logger := slogx.New(slogx.Options{
				Level:      slog.LevelDebug,
				Format:     slogx.TextColored,
				TimeLayout: "2006-01-02 15:04:05.000000",
				Writer:     &output,
			})
			record := slog.NewRecord(testTimestamp(123456789), test.level, "probe", 0)
			if err := logger.Handler().Handle(context.Background(), record); err != nil {
				t.Fatalf("handle record: %v", err)
			}

			actual := output.String()
			t.Logf("slogx output: %s", strings.TrimSpace(actual))
			expected := "level=" + test.color + test.level.String() + "\x1b[0m"
			if !strings.Contains(actual, expected) {
				t.Fatalf("output = %q, want colored level %q", actual, expected)
			}
		})
	}
}

func TestTextColoredLeavesCustomLevelsUncolored(t *testing.T) {
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{
		Level:      slog.LevelDebug,
		Format:     slogx.TextColored,
		TimeLayout: "2006-01-02 15:04:05.000000",
		Writer:     &output,
	})
	record := slog.NewRecord(testTimestamp(0), slog.LevelInfo+1, "probe", 0)
	if err := logger.Handler().Handle(context.Background(), record); err != nil {
		t.Fatalf("handle record: %v", err)
	}

	actual := output.String()
	t.Logf("slogx output: %s", strings.TrimSpace(actual))
	if strings.Contains(actual, "\x1b[") {
		t.Fatalf("custom level was colored: %q", actual)
	}
	if !strings.Contains(actual, "level=INFO+1") {
		t.Fatalf("output = %q, want custom level", actual)
	}
}

func TestTextColoredPreservesMessageWithSpaces(t *testing.T) {
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{
		Format:     slogx.TextColored,
		TimeLayout: "2006-01-02 15:04:05.000000",
		Writer:     &output,
	})
	record := slog.NewRecord(testTimestamp(123456789), slog.LevelInfo, "probe with spaces", 0)
	if err := logger.Handler().Handle(context.Background(), record); err != nil {
		t.Fatalf("handle record: %v", err)
	}

	actual := output.String()
	t.Logf("slogx output: %s", strings.TrimSpace(actual))
	if !strings.Contains(actual, "level=\x1b[32mINFO\x1b[0m") {
		t.Fatalf("output = %q, want colored INFO level", actual)
	}
	if !strings.Contains(actual, `msg="probe with spaces"`) {
		t.Fatalf("output = %q, want quoted message", actual)
	}
}
