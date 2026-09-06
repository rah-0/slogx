package slogx

import "errors"

// Traceparent validation errors returned by [ParseTraceparent].
// Use [errors.Is] to distinguish the invalid component.
var (
	ErrInvalidTraceparentLength  = errors.New("slogx: invalid traceparent length")
	ErrInvalidTraceparentVersion = errors.New("slogx: invalid traceparent version")
	ErrInvalidTraceparentTraceID = errors.New("slogx: invalid traceparent trace ID")
	ErrInvalidTraceparentSpanID  = errors.New("slogx: invalid traceparent span ID")
	ErrInvalidTraceparentFlags   = errors.New("slogx: invalid traceparent flags")
)

// JSON record errors returned by [JSONDecoder.Decode]. Standard-library and
// reader errors retain their identity and can also be inspected with errors.Is
// or errors.As.
var (
	ErrJSONRecordNotObject  = errors.New("slogx: JSON record must be an object")
	ErrInvalidJSONFieldName = errors.New("slogx: JSON record field name must be a string")
	// ErrJSONFieldDecode identifies an invalid built-in field. The returned
	// error also wraps the underlying decoding error and includes the field name.
	ErrJSONFieldDecode = errors.New("slogx: decode JSON record field")
)

const (
	errJSONFieldDecodeFormat = "%w %q: %w"
	wrappedErrorSeparator    = ": "
)
