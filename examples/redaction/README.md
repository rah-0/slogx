# Attribute redaction

Run from the repository root:

```sh
go run ./examples/redaction
```

`ReplaceAttr` receives resolved slog attributes and their group path. Returning
an empty `slog.Attr` removes a field. The example removes `password` and `token`
fields, redacts only `request.auth.email`, and retains `request.contact.email`.
This applies to fields bound with `With` and fields supplied to the log call;
the surrounding native groups remain intact.

The callback visits native slog groups. It does not walk inside an arbitrary
map or struct passed as a single `slog.Any` value; redact those values before
logging them or handle the containing attribute explicitly.

When `TimeLayout` is configured, `ReplaceAttr` runs first. Timestamp formatting
applies only to returned root attributes named `time` whose values remain times.
Removing, renaming, or replacing that value with a string preserves the callback's
choice.
