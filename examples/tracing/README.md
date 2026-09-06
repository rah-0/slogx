# Three-layer tracing

Run from the repository root:

```sh
go run ./examples/tracing
```

[main.go](main.go) runs `handleRequest → loadItem → queryItem`. The simulated
database failure returns through `Wrap` at every layer. The request boundary logs
it once, and the program exits successfully. Each deferred `End` writes its
completed span through the same JSON logger. [main_test.go](main_test.go) checks
the three-span parent chain, per-layer attributes, error events, and source fields.

Pass the context returned by `StartSpan` to child calls and context-aware log
calls. Slogx loggers attach its `trace_id` and `span_id`; no `request_id` is required.
Span attributes describe one operation and are not inherited by ordinary logs.
Use [WithAttrs](../context) for metadata that should accompany those logs.

`RecordError` adds an `error` event with a structured `err` attribute; it neither
writes a separate log nor changes status. `RecordError(nil)` does nothing.
`SetStatus` accepts `StatusUnset`, `StatusOK`, and `StatusError`; the last valid call
wins, and only `StatusError` retains a description. A recovered error can remain
an event in a successful span.

`SetAttrs` and `RecordError` preserve attribute order and duplicate keys, resolve
LogValuers when called, and copy attribute/group structure. Values held by
`slog.Any` remain shallow references and must be safe to read until logging ends.

The default logger and caller are captured at `StartSpan`. Native handlers apply
`AddSource`, `ReplaceAttr`, and grouping to the completion record and nested
attributes. `source.function`, `source.file`, and `source.line` describe the
`StartSpan` caller; they do not provide a full stack or the origin of an underlying
error. Changing the default logger does not redirect an existing span.

Roots are sampled; children inherit their parent's sampled flag and trace state.
Recording requires sampling and an `INFO`-enabled logger at the start. Other spans
still carry IDs but do not retain attributes or events. `IsRecording` lets callers
skip expensive attribute preparation. Completion checks `INFO` again.

`End` is synchronous and emits at most one record. Concurrent and repeated calls
wait for the same completion; later mutations do nothing. It keeps the span's
context values while detaching cancellation and deadlines. `EndContext(ctx)` uses
the supplied context unchanged; nil uses `context.Background()`. Neither method
returns a handler error or changes the caller's context. There is no background
queue or shutdown/flush API. `SpanFromContext` retrieves the current local span.

## Record format

Each completion is an ordinary slog record. Its reserved `span` group contains
`trace_id`, `span_id`, `parent_span_id`, `trace_flags`, `trace_state`, `name`, `kind`,
`start_time`, `end_time`, `status`, and `status_message`. Absent optional fields are
omitted. `span.events` contains ordered groups named `0`, `1`, and so on; each event
contains `time`, `msg`, and `attrs`.

Context, correlation, and operation attributes follow the logger's active
`WithGroup`. To send spans to the separate
[collector executable](https://github.com/rah-0/slogx-collector#traces), preserve the
`span` group's names and types at the record root. Group application fields with
`slog.Group` rather than putting the default logger under `WithGroup`.

The collector converts and delivers the JSON records and routes ordinary logs
separately. Applications import only slogx; it has no OTLP encoding, HTTP delivery,
or collector dependency. A logging call does not acknowledge durable storage;
durability starts when the collector writes and syncs its journal.
