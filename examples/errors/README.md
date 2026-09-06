# Structured errors through three layers

Run from the repository root:

```sh
go run ./examples/errors
```

`handleRequest` calls `loadItem`, which calls `queryItem`. Each function adds
attributes with `slogx.Wrap`; the request boundary logs one ERROR record. The
`err` field retains each layer's `msg`, `attrs`, and `cause`, including the
nested `http` and `db` groups. No context is needed.

`Wrap` preserves the cause through `Unwrap`, so `errors.Is` and `errors.As`
continue to work. It implements `slog.LogValuer`, allowing native JSON and text
handlers to render the structure. Use `Wrap` at each structured layer: an
intervening `fmt.Errorf` or `errors.Join` retains the Go error chain but hides
inner `LogValuer` values from slog, which then renders that error as a string.

`Wrap(nil, ...)` returns nil. `Error` and `ErrorContext` still emit an enabled
ERROR record for a nil error, with `err: null` in JSON. Empty attribute groups
are omitted. Values are shallow references and must remain safe to read when
the error is logged.
