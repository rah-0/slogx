# slogx

`slogx` is a small, opinionated layer over Go's [`log/slog`](https://pkg.go.dev/log/slog).
It configures the standard slog handlers instead of replacing slog's types or logging model.

- Returns genuine `*slog.Logger` values.
- Provides practical defaults for timestamps and output, with optional source reporting.
- Supports text, JSON, colored text, and systemd journal output.
- Supports runtime level changes through `slog.LevelVar`.
- Decodes its JSON logs without losing attribute order or duplicate keys.
- Uses only the Go standard library.

## Requirements

Go 1.27 or later.

## Installation

```sh
go get github.com/rah-0/slogx
```

For collecting JSON logs, see the separate
[`slogx-collector` repository](https://github.com/rah-0/slogx-collector). Its executable reads
from a pipe and delivers logs through a disk journal and pluggable destinations.

## Quick start

Configure slog once through Slogx, then continue using the standard `log/slog` package:

```go
package main

import (
	"log/slog"

	"github.com/rah-0/slogx"
)

func main() {
	slogx.SetDefault(slogx.Options{
		AddSource: true,
		Format:    slogx.TextColored,
	})

	slog.Info("service started", "version", "1.0.0")
}
```

`SetDefault` installs a native `*slog.Logger` as slog's process-wide default. Calls through
`slog.Debug`, `slog.Info`, `slog.Warn`, `slog.Error`, and the logger returned by `slog.Default()` all
use the configured handler.

This configuration produces output like this, with the level name colored in a terminal:

```text
time="2026-09-03 20:45:12.123456" level=INFO source=/path/main.go:16 msg="service started" version=1.0.0
```

## Constructors

| Function | Purpose |
| --- | --- |
| `New` | Builds a logger with slog-like defaults: text output, `INFO` level, stderr, native timestamp formatting, and source reporting only when requested. |
| `NewDefault` | Builds a logger with Slogx defaults: stdout and UTC microsecond timestamps. |
| `SetDefault` | Builds a logger with `NewDefault` and installs it through `slog.SetDefault`. |

Use an independent logger when a process needs more than one configuration:

```go
logger := slogx.NewDefault(slogx.Options{
	Format: slogx.JSON,
})

logger.Info("request completed", "status", 200)
```

Because `slogx.Logger` aliases `slog.Logger`, the result works anywhere a `*slog.Logger` is expected.
Standard methods such as `With`, `Log`, and `LogAttrs` remain available without adapters. Use `New`
when native timestamps or stderr output are preferred over the Slogx defaults.

## Options

| Option | Behaviour |
| --- | --- |
| `Format` | Selects `Text`, `JSON`, `TextColored`, or `Systemd`. The zero value is `Text`. |
| `Level` | Sets the minimum enabled level. The default is `INFO`; a `*slog.LevelVar` can change it at runtime. |
| `Writer` | Receives the output. `New` defaults to stderr; `NewDefault` and `SetDefault` default to stdout. |
| `AddSource` | Adds the calling file and line when true. The default is false. |
| `TimeLayout` | Formats timestamps in UTC using Go's time layout syntax. With `New`, an empty value preserves slog's native timestamp format. |

`NewDefault` uses this layout when `TimeLayout` is empty:

```text
2006-01-02 15:04:05.000000
```

It produces fixed-width UTC timestamps such as:

```text
2026-09-03 20:45:12.123456
```

Provide another Go layout to override it:

```go
logger := slogx.NewDefault(slogx.Options{
	TimeLayout: "2006/01/02-15:04:05",
})
```

## Formats

| Format | Output |
| --- | --- |
| `Text` | Standard `slog.TextHandler` output. |
| `JSON` | Standard `slog.JSONHandler` output. |
| `TextColored` | Text output with only the standard level name colored. Custom levels remain uncolored. |
| `Systemd` | Single-line text output prefixed with a systemd journal priority. It is intentionally uncolored. |

`TextColored` always writes ANSI escape sequences; it does not detect whether the writer is a
terminal.

Text:

```text
time="2026-09-03 20:45:12.123456" level=INFO msg="service started" version=1.0.0
```

JSON:

```json
{"time":"2026-09-03 20:45:12.123456","level":"INFO","msg":"service started","version":"1.0.0"}
```

## Runtime log levels

`Options.Level` accepts slog's `Leveler` interface, so `slog.LevelVar` works directly:

```go
var level slog.LevelVar
level.Set(slog.LevelInfo)

slogx.SetDefault(slogx.Options{
	Level: &level,
})

level.Set(slog.LevelDebug)
slog.Debug("debug logging enabled")
```

## Error logging

Use these helpers when a log event carries an `error`:

```go
func Error(msg string, err error, args ...any)
func ErrorCtx(ctx context.Context, msg string, err error, args ...any)
```

For example:

```go
slogx.Error("request failed", err, "status_code", 503)
slogx.ErrorCtx(ctx, "request failed", err, "status_code", 503)
```

`msg` remains the log record's message, while `err` is added as the structured `"err"` attribute.
Any additional arguments are added with slog's native structured-attribute semantics. A nil error is
logged as a structured nil value; the helpers do not silently discard the event. `ErrorCtx` passes
its context to the handler unchanged.

The helpers write through `slog.Default()`, so they use the logger and configuration installed by
`SetDefault`. Standard calls such as `slog.Error` remain available, including for error-level events
that do not carry an `error` value. Caller source is emitted only when `AddSource` is enabled. `New`,
`NewDefault`, and `SetDefault` all honor `Options.AddSource`.

## JSON decoding

`NewJSONDecoder` reads the newline-delimited records produced by the `JSON` format:

```go
record, err := slogx.NewJSONDecoder(reader).Decode()
if err != nil {
	return err
}

fmt.Println(record.Time, record.Level, record.Message)
```

`Decode` returns `io.EOF` after the final record. `JSONRecord.Attributes` retains application
attributes in their original order. Repeated keys are preserved, and each value remains a
`json.RawMessage`, so large integers and nested values are not silently converted.

## systemd journal

Use `Systemd` for services whose standard output or error is captured by the systemd journal. It
uses slog's text handler without ANSI colors and adds a journal priority prefix to each record:

```go
slogx.SetDefault(slogx.Options{
	AddSource: true,
	Format:    slogx.Systemd,
})

slog.Info("service started")
```

Configure the unit to capture both output streams and interpret priority prefixes:

```ini
[Service]
StandardOutput=journal
StandardError=journal
SyslogLevelPrefix=yes
```

Slog levels map to journal priorities as follows:

| slog level range | Journal priority |
| --- | ---: |
| Below `INFO` | `DEBUG` (`7`) |
| `INFO` to below `WARN` | `INFO` (`6`) |
| `WARN` to below `ERROR` | `WARNING` (`4`) |
| `ERROR` and above | `ERR` (`3`) |

Before journal ingestion, a warning record begins with its priority prefix:

```text
<4>time="2026-09-03 20:45:12.123456" level=WARN source=/path/main.go:20 msg="disk space is low"
```

systemd removes `<4>` and stores `PRIORITY=4` alongside the remaining message. Cockpit and
`journalctl` can then filter the service by severity:

```sh
journalctl --unit=my-service.service --priority=warning
```

Priority filtering is inclusive, so `warning` returns both `WARN` and `ERROR` records. The text
handler escapes embedded newlines, keeping each log event on one physical journal line. Use this
format only for journal-connected streams; otherwise the `<N>` prefix remains visible.

## Tests

Run `go test -race ./...` from the repository root.

## License

Slogx is available under the [MIT License](LICENSE).

## Support

If `slogx` saves you time, you can support its development:

[![Buy Me A Coffee](https://cdn.buymeacoffee.com/buttons/default-orange.png)](https://www.buymeacoffee.com/rah.0)
