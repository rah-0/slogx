package slogx

import "log/slog"

// Wrap adds a message and structured attributes to err. A nil err returns nil.
// Arguments follow [slog.Logger.With] conventions. Attribute values are not
// deep-copied and must be safe to read when the error is logged.
//
// The returned error unwraps to err and implements [slog.LogValuer]. Native slog
// handlers render each layer as a group with msg, attrs, and cause fields; empty
// attrs groups are omitted. Use Wrap at each layer to retain structured values:
// ordinary fmt.Errorf wrappers and errors.Join do not expose inner LogValuers.
func Wrap(err error, msg string, args ...any) error {
	if err == nil {
		return nil
	}
	return &wrappedError{
		cause: err,
		msg:   msg,
		attrs: slog.Group("", args...).Value.Group(),
	}
}

// With adds structured attributes to err without changing its error text.
// It is equivalent to Wrap(err, "", args...) and returns nil when err is nil.
func With(err error, args ...any) error {
	return Wrap(err, "", args...)
}

type wrappedError struct {
	cause error
	msg   string
	attrs []slog.Attr
}

func (err *wrappedError) Error() string {
	if err.msg == "" {
		return err.cause.Error()
	}
	return err.msg + wrappedErrorSeparator + err.cause.Error()
}

func (err *wrappedError) Unwrap() error {
	return err.cause
}

func (err *wrappedError) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("msg", err.msg),
		slog.GroupAttrs("attrs", err.attrs...),
		slog.Any("cause", err.cause),
	)
}
