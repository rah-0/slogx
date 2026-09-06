package slogx_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/rah-0/slogx"
)

func TestWrapPreservesErrorChain(t *testing.T) {
	cause := &itemError{id: 42}
	inner := slogx.Wrap(cause, "query item", "attempt", 2)
	outer := slogx.Wrap(inner, "load item", "item_id", 42)

	if got := outer.Error(); got != "load item: query item: item unavailable" {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(inner, errors.Unwrap(outer)) || !errors.Is(cause, errors.Unwrap(inner)) {
		t.Fatal("Unwrap did not preserve the immediate causes")
	}
	if !errors.Is(outer, cause) {
		t.Fatal("errors.Is did not find the underlying cause")
	}
	var matched *itemError
	if !errors.As(outer, &matched) || !errors.Is(matched, cause) {
		t.Fatal("errors.As did not recover the original typed error")
	}
	if !errors.Is(fmt.Errorf("request failed: %w", outer), cause) {
		t.Fatal("ordinary error wrapping lost the underlying cause")
	}
}

func TestWrapNilAndEmptyMessage(t *testing.T) {
	if err := slogx.Wrap(nil, "load item", "item_id", 42); err != nil {
		t.Fatalf("Wrap(nil) = %v, want nil", err)
	}
	cause := errors.New("connection refused")
	if got := slogx.Wrap(cause, "", "attempt", 2).Error(); got != cause.Error() {
		t.Fatalf("empty-message Error() = %q, want %q", got, cause.Error())
	}
}

func TestWrapJSONPreservesEveryLayer(t *testing.T) {
	cause := errors.New("connection refused")
	err := slogx.Wrap(cause, "query item",
		"id", 42, "attempt", 2, "retryable", true,
		"msg", "application value", "cause", "application cause",
	)
	err = slogx.Wrap(err, "load item", "id", 84)
	err = slogx.Wrap(err, "handle request", "id", "req-123",
		slog.Group("http", "method", "GET", "path", "/items/42"),
	)

	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			var output bytes.Buffer
			if native {
				slog.New(slog.NewJSONHandler(&output, nil)).Error("request failed", "err", err)
			} else {
				setTestDefault(t, slogx.New(slogx.Options{Format: slogx.JSON, Writer: &output}))
				slogx.Error("request failed", err)
			}

			record, decodeErr := slogx.NewJSONDecoder(&output).Decode()
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			var got, want any
			if decodeErr := json.Unmarshal(errorAttributeValue(t, record.Attributes, "err"), &got); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			const expected = `{
				"msg": "handle request",
				"attrs": {"id": "req-123", "http": {"method": "GET", "path": "/items/42"}},
				"cause": {
					"msg": "load item", "attrs": {"id": 84},
					"cause": {
						"msg": "query item",
						"attrs": {"id": 42, "attempt": 2, "retryable": true,
							"msg": "application value", "cause": "application cause"},
						"cause": "connection refused"
					}
				}
			}`
			if decodeErr := json.Unmarshal([]byte(expected), &want); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("structured error = %#v, want %#v", got, want)
			}
		})
	}
}

func TestWrapUsesNativeAttributeSemantics(t *testing.T) {
	args := []any{slog.String("key", "first"), "key", "second", "dangling"}
	err := slogx.Wrap(errors.New("failure"), "operation", args...)
	args[0] = slog.String("key", "changed")
	args[2] = "changed"

	var output bytes.Buffer
	slog.New(slog.NewJSONHandler(&output, nil)).Error("failed", "err", err)
	record, decodeErr := slogx.NewJSONDecoder(&output).Decode()
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	const want = `{"msg":"operation","attrs":{"key":"first","key":"second","!BADKEY":"dangling"},"cause":"failure"}`
	if got := string(errorAttributeValue(t, record.Attributes, "err")); got != want {
		t.Fatalf("err = %s, want %s", got, want)
	}
}

func TestWrapWithoutAttributes(t *testing.T) {
	var output bytes.Buffer
	slog.New(slog.NewJSONHandler(&output, nil)).Error("failed", "err",
		slogx.Wrap(errors.New("failure"), "operation"))
	record, err := slogx.NewJSONDecoder(&output).Decode()
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"msg":"operation","cause":"failure"}`
	if got := string(errorAttributeValue(t, record.Attributes, "err")); got != want {
		t.Fatalf("err = %s, want %s", got, want)
	}
}

func TestWrapWorksWithNativeTextHandler(t *testing.T) {
	var output bytes.Buffer
	err := slogx.Wrap(slogx.Wrap(errors.New("failure"), "query", "attempt", 2), "load", "id", 42)
	slog.New(slog.NewTextHandler(&output, nil)).Error("failed", "err", err)
	for _, field := range []string{"err.msg=load", "err.attrs.id=42", "err.cause.msg=query", "err.cause.attrs.attempt=2", "err.cause.cause=failure"} {
		if !strings.Contains(output.String(), field) {
			t.Fatalf("output %q does not contain %q", output.String(), field)
		}
	}
}

type itemError struct {
	id int
}

func (*itemError) Error() string { return "item unavailable" }
