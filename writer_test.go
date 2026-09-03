package slogx_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/rah-0/slogx"
)

func TestDecoratedFormatsReportWriterFailures(t *testing.T) {
	writeError := errors.New("write failed")
	for _, format := range []struct {
		name  string
		value slogx.Format
	}{
		{name: "colored text", value: slogx.TextColored},
		{name: "systemd", value: slogx.Systemd},
	} {
		for _, test := range []struct {
			name   string
			writer writerFunc
			want   error
		}{
			{
				name: "error",
				writer: func([]byte) (int, error) {
					return 0, writeError
				},
				want: writeError,
			},
			{
				name: "short write",
				writer: func(record []byte) (int, error) {
					return len(record) - 1, nil
				},
				want: io.ErrShortWrite,
			},
		} {
			t.Run(format.name+"/"+test.name, func(t *testing.T) {
				logger := slogx.New(slogx.Options{
					Format: format.value,
					Writer: test.writer,
				})
				record := slog.NewRecord(testTimestamp(0), slog.LevelInfo, "probe", 0)
				err := logger.Handler().Handle(context.Background(), record)
				if !errors.Is(err, test.want) {
					t.Fatalf("handle error = %v, want %v", err, test.want)
				}
			})
		}
	}
}

type writerFunc func([]byte) (int, error)

func (write writerFunc) Write(record []byte) (int, error) {
	return write(record)
}
