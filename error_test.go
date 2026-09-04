package slogx_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"runtime"
	"testing"

	"github.com/rah-0/slogx"
)

func TestErrorWritesStructuredRecord(t *testing.T) {
	var output bytes.Buffer
	logger := slogx.New(slogx.Options{
		Format: slogx.JSON,
		Writer: &output,
	}).With("component", "mailer")
	setTestDefault(t, logger)

	slogx.Error(
		"postmark request failed",
		errors.New("connection refused"),
		"status_code", 503,
		slog.String("provider", "postmark"),
	)

	record, err := slogx.NewJSONDecoder(&output).Decode()
	if err != nil {
		t.Fatalf("decode JSON output: %v", err)
	}
	if record.Level != slog.LevelError {
		t.Fatalf("level = %s, want %s", record.Level, slog.LevelError)
	}
	if record.Message != "postmark request failed" {
		t.Fatalf("message = %q, want %q", record.Message, "postmark request failed")
	}
	if actual := string(errorAttributeValue(t, record.Attributes, "component")); actual != `"mailer"` {
		t.Fatalf("component = %s, want %q", actual, "mailer")
	}
	if actual := string(errorAttributeValue(t, record.Attributes, "err")); actual != `"connection refused"` {
		t.Fatalf("err = %s, want %q", actual, "connection refused")
	}
	if actual := string(errorAttributeValue(t, record.Attributes, "status_code")); actual != "503" {
		t.Fatalf("status_code = %s, want 503", actual)
	}
	if actual := string(errorAttributeValue(t, record.Attributes, "provider")); actual != `"postmark"` {
		t.Fatalf("provider = %s, want %q", actual, "postmark")
	}
}

func TestErrorPreservesErrorValue(t *testing.T) {
	cause := errors.New("connection refused")
	handler := new(captureHandler)
	setTestDefault(t, slog.New(handler))

	slogx.Error("postmark request failed", cause, "status_code", 503)

	var attributes []slog.Attr
	handler.handledRecord.Attrs(func(attribute slog.Attr) bool {
		attributes = append(attributes, attribute)
		return true
	})
	if len(attributes) != 2 {
		t.Fatalf("attribute count = %d, want 2", len(attributes))
	}
	if attributes[0].Key != "err" {
		t.Fatalf("first attribute key = %q, want err", attributes[0].Key)
	}
	if actual := attributes[0].Value.Any(); actual != cause {
		t.Fatalf("error attribute = %v, want original error", actual)
	}
	if attributes[1].Key != "status_code" || attributes[1].Value.Int64() != 503 {
		t.Fatalf("second attribute = %v, want status_code=503", attributes[1])
	}
}

func TestErrorLogsNilError(t *testing.T) {
	for _, test := range []struct {
		name string
		log  func()
	}{
		{name: "Error", log: func() { slogx.Error("ignored", nil, "key", "value") }},
		{name: "ErrorCtx", log: func() { slogx.ErrorCtx(context.Background(), "ignored", nil, "key", "value") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			setTestDefault(t, slogx.New(slogx.Options{
				Format: slogx.JSON,
				Writer: &output,
			}))

			test.log()
			record, err := slogx.NewJSONDecoder(&output).Decode()
			if err != nil {
				t.Fatalf("decode JSON output: %v", err)
			}
			if actual := string(errorAttributeValue(t, record.Attributes, "err")); actual != "null" {
				t.Fatalf("err = %s, want null", actual)
			}
		})
	}
}

func TestErrorUsesBackgroundContext(t *testing.T) {
	handler := new(captureHandler)
	setTestDefault(t, slog.New(handler))

	slogx.Error("request failed", errors.New("connection refused"))

	if handler.enabledContext != context.Background() {
		t.Fatal("Enabled did not receive a background context")
	}
	if handler.handledContext != context.Background() {
		t.Fatal("Handle did not receive a background context")
	}
}

func TestErrorCtxPropagatesContext(t *testing.T) {
	handler := new(captureHandler)
	setTestDefault(t, slog.New(handler))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	slogx.ErrorCtx(ctx, "request failed", errors.New("connection refused"))

	if handler.enabledContext != ctx {
		t.Fatal("Enabled did not receive the supplied context")
	}
	if handler.handledContext != ctx {
		t.Fatal("Handle did not receive the supplied context")
	}
}

func TestErrorCtxPassesNilContextThrough(t *testing.T) {
	handler := new(captureHandler)
	setTestDefault(t, slog.New(handler))

	slogx.ErrorCtx(nil, "request failed", errors.New("connection refused"))

	if handler.enabledContext != nil {
		t.Fatal("Enabled context was replaced")
	}
	if handler.handledContext != nil {
		t.Fatal("Handle context was replaced")
	}
}

func TestErrorHonorsDisabledLevel(t *testing.T) {
	for _, test := range []struct {
		name string
		log  func()
	}{
		{name: "Error", log: func() { slogx.Error("hidden", errors.New("failure")) }},
		{name: "ErrorCtx", log: func() { slogx.ErrorCtx(context.Background(), "hidden", errors.New("failure")) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			setTestDefault(t, slogx.New(slogx.Options{
				Level:  slog.LevelError + 1,
				Format: slogx.JSON,
				Writer: &output,
			}))

			test.log()
			if output.Len() != 0 {
				t.Fatalf("output = %q, want empty", output.String())
			}
		})
	}
}

func TestErrorReportsCallerSource(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		handler := new(captureHandler)
		setTestDefault(t, slog.New(handler))

		slogx.Error("source probe", errors.New("failure"))
		expected := sourceImmediatelyBefore(t)

		assertErrorSource(t, handler.handledRecord.Source(), expected)
	})

	t.Run("ErrorCtx", func(t *testing.T) {
		handler := new(captureHandler)
		setTestDefault(t, slog.New(handler))

		slogx.ErrorCtx(context.Background(), "source probe", errors.New("failure"))
		expected := sourceImmediatelyBefore(t)

		assertErrorSource(t, handler.handledRecord.Source(), expected)
	})
}

func TestErrorHonorsAddSource(t *testing.T) {
	for _, source := range []struct {
		name    string
		enabled bool
	}{
		{name: "disabled"},
		{name: "enabled", enabled: true},
	} {
		t.Run(source.name, func(t *testing.T) {
			for _, test := range []struct {
				name string
				log  func(*testing.T) slog.Source
			}{
				{name: "Error", log: func(t *testing.T) slog.Source {
					slogx.Error("source probe", errors.New("failure"))
					return sourceImmediatelyBefore(t)
				}},
				{name: "ErrorCtx", log: func(t *testing.T) slog.Source {
					slogx.ErrorCtx(context.Background(), "source probe", errors.New("failure"))
					return sourceImmediatelyBefore(t)
				}},
			} {
				t.Run(test.name, func(t *testing.T) {
					var output bytes.Buffer
					setTestDefault(t, slogx.New(slogx.Options{
						AddSource: source.enabled,
						Format:    slogx.JSON,
						Writer:    &output,
					}))

					expected := test.log(t)
					record, err := slogx.NewJSONDecoder(&output).Decode()
					if err != nil {
						t.Fatalf("decode JSON output: %v", err)
					}
					if !source.enabled {
						if record.Source != nil {
							t.Fatalf("source = %+v, want nil", *record.Source)
						}
						return
					}
					assertErrorSource(t, record.Source, expected)
				})
			}
		})
	}
}

type captureHandler struct {
	enabledContext context.Context
	handledContext context.Context
	handledRecord  slog.Record
}

func (handler *captureHandler) Enabled(ctx context.Context, _ slog.Level) bool {
	handler.enabledContext = ctx
	return true
}

func (handler *captureHandler) Handle(ctx context.Context, record slog.Record) error {
	handler.handledContext = ctx
	handler.handledRecord = record.Clone()
	return nil
}

func (handler *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return handler
}

func (handler *captureHandler) WithGroup(_ string) slog.Handler {
	return handler
}

func setTestDefault(t *testing.T, logger *slog.Logger) {
	t.Helper()
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
}

func sourceImmediatelyBefore(t *testing.T) slog.Source {
	t.Helper()
	pc, file, line, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("resolve caller source")
	}
	function := runtime.FuncForPC(pc)
	if function == nil {
		t.Fatal("resolve caller function")
	}
	return slog.Source{
		Function: function.Name(),
		File:     file,
		Line:     line - 1,
	}
}

func assertErrorSource(t *testing.T, actual *slog.Source, expected slog.Source) {
	t.Helper()
	if actual == nil {
		t.Fatal("source is nil")
	}
	if *actual != expected {
		t.Fatalf("source = %+v, want external caller %+v", *actual, expected)
	}
}

func errorAttributeValue(t *testing.T, attributes []slogx.JSONAttr, key string) []byte {
	t.Helper()
	for _, attribute := range attributes {
		if attribute.Key == key {
			return attribute.Value
		}
	}
	t.Fatalf("attribute %q not found", key)
	return nil
}
