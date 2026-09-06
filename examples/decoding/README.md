# Reading native JSON log records

Run from the repository root:

```sh
go run ./examples/decoding
```

The example reads two local records with `NewJSONDecoder` and prints each
attribute in order. The repeated `tag` fields remain separate, the nested
object stays raw JSON, and `9007199254740993` retains its exact digits.

`JSONRecord.Attributes` is an ordered slice of `JSONAttr`, whose `Value` is
`json.RawMessage`. Decode an individual value only when its type is needed;
converting all values to `float64` can round large integers.

The decoder follows native `slog.JSONHandler` field ordering. It treats
`time`, `level`, `source`, and the first `msg` as built-ins. Every field after
that first `msg` becomes an attribute, including later `msg` and `level`
fields. Before `msg`, repeated built-in fields replace their earlier decoded
values.

If `ReplaceAttr` renames or removes the built-in `msg`, the first remaining
`msg` still defines this boundary. Renamed fields are treated as attributes
unless their new names match a built-in before that boundary.

`io.EOF` means clean end of input between records. Truncated JSON returns a
non-EOF error, so the loop does not silently accept an incomplete last record.
Use `errors.Is` to distinguish `slogx.ErrJSONRecordNotObject` and
`slogx.ErrJSONFieldDecode`; the latter also wraps the underlying decode error.
