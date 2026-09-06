# Context attributes through three calls

Run from the repository root:

```sh
go run ./examples/context
```

`WithAttrs` accumulates metadata as `handleRequest` calls `loadItem` and then
`queryItem`. The innermost log contains attributes from all three layers.
The returned error is logged with `ErrorContext` at the request boundary;
child context attributes do not travel upward with that error. Use structured
error wrapping when values must travel upward with a failure.

The final two records demonstrate that deriving a context changes neither its
parent nor its siblings. `request_id` is an optional field chosen by the
application; this example creates no spans.

Use a logger created by `slogx.New`, `NewDefault`, or `SetDefault` and pass the
context to a logging method. Other context values are not collected
automatically. Context attributes follow logger-bound attributes, precede
call-site attributes, and remain inside the logger's active `WithGroup`.
Duplicate keys retain native slog behavior.

`WithAttrs` preserves cancellation and deadlines. It copies attribute slices,
but maps, pointers, and other values remain shallow references and must not be
mutated concurrently with logging. A nil context uses `context.Background`.
