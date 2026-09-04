package slogx

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// Error logs msg at [slog.LevelError] with err as a structured "err" attribute.
// Additional arguments are handled as in [slog.Logger.Error].
func Error(msg string, err error, args ...any) {
	logError(context.Background(), msg, err, args...)
}

// ErrorCtx is like [Error] but uses ctx.
func ErrorCtx(ctx context.Context, msg string, err error, args ...any) {
	logError(ctx, msg, err, args...)
}

func logError(ctx context.Context, msg string, err error, args ...any) {
	handler := slog.Default().Handler()
	if !handler.Enabled(ctx, slog.LevelError) {
		return
	}

	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	record := slog.NewRecord(time.Now(), slog.LevelError, msg, pcs[0])
	record.AddAttrs(slog.Any("err", err))
	record.Add(args...)
	_ = handler.Handle(ctx, record)
}
