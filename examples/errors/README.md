# Structured errors through three layers

Run from the repository root:

```sh
go run ./examples/errors
```

`handleRequest` calls `loadItem`, which calls `queryItem`. Each function adds
attributes with `slogx.Wrap`; the request boundary logs one ERROR record. The
`err` field retains each layer's `msg`, `attrs`, and `cause`, including the
nested `http` and `db` groups. No context is needed.

Use `slogx.With` to add attributes without changing the error's `Error()` text:

```go
return slogx.With(err,
	"operation", "load_item",
	"item_id", itemID,
)
```

`With(err, args...)` is equivalent to `Wrap(err, "", args...)`. It retains the
same structured layer, including an empty `msg` field.

Both helpers preserve the cause through `Unwrap`, so `errors.Is` and `errors.As`
continue to work. Their errors implement `slog.LogValuer`, allowing native JSON
and text handlers to render the structure. Use `Wrap` or `With` at each structured
layer: an intervening `fmt.Errorf` or `errors.Join` retains the Go error chain but
hides inner `LogValuer` values from slog, which then renders that error as a string.

`Wrap(nil, ...)` and `With(nil, ...)` return nil. `Error` and `ErrorContext` still
emit an enabled ERROR record for a nil error, with `err: null` in JSON. Empty
attribute groups are omitted. Values are shallow references and must remain safe
to read when the error is logged.
