# slogx

`slogx` configures Go's [`log/slog`](https://pkg.go.dev/log/slog) and adds structured
error wrapping, context attributes, and spans. It returns native `*slog.Logger`
values and uses only the Go standard library.

Requires Go 1.27.1 or later.

```sh
go get github.com/rah-0/slogx
```

## Quick start

Configure the default logger once, then use the standard `slog` methods:

```go
package main

import (
	"log/slog"

	"github.com/rah-0/slogx"
)

func main() {
	slogx.SetDefault(slogx.Options{
		Format:    slogx.JSON,
		AddSource: true,
	})

	slog.Info("service started", "service", "catalog", "version", "1.0.0")
}
```

## Examples

The [examples directory](examples/README.md) contains a runnable program and tests
for each use case: structured fields, output formats, runtime levels, error chains,
context attributes, redaction, spans, HTTP propagation, identifiers, and JSON decoding.

Run a case from the repository root:

```sh
go run ./examples/basic
go test -count=1 -race -cover -covermode=atomic ./examples/basic
```

Each example has a short README explaining its output and relevant behavior.
They share the root module and require no external services.

## Configuration

| Constructor | Defaults |
| --- | --- |
| `New` | Text, `INFO`, stderr, native slog timestamps. |
| `NewDefault` | Text, `INFO`, stdout, UTC timestamps using `2006-01-02 15:04:05.000000`. |
| `SetDefault` | Installs `NewDefault` through `slog.SetDefault`. |

All constructors accept [Options](logger.go):

| Option | Purpose |
| --- | --- |
| `Format` | `Text`, `JSON`, `TextColored`, or `Systemd`. Zero and unsupported values use `Text`. |
| `Level` | Minimum enabled level; accepts `*slog.LevelVar` for runtime changes. |
| `Writer` | Output destination. |
| `AddSource` | Native caller information; disabled by default. For spans, identifies the `StartSpan` caller. |
| `ReplaceAttr` | Native slog filtering, redaction, and renaming callback. |
| `TimeLayout` | Custom UTC timestamp layout. With `New`, an empty value preserves native formatting. |

Standard methods such as `With`, `WithGroup`, `LogAttrs`, and `InfoContext` remain
available. Grouping, duplicate keys, and attribute formatting follow native slog.
See [formats](examples/formats) and [redaction](examples/redaction) for timestamp,
color, systemd, and callback details.

## Structured errors, context, and spans

- [`Wrap`](examples/errors) retains a message and attributes at each error layer,
  preserving `errors.Is` and `errors.As`. `Error` and `ErrorContext` log the error
  under the structured `err` attribute.
- [`WithAttrs`](examples/context) explicitly attaches metadata to context-aware
  log calls. Other context values and local variables are not collected automatically.
- [`StartSpan`](examples/tracing) creates an operation and a context for correlated
  logs and child spans. Recording spans emit at `INFO` when the captured logger still enables it.
  [HTTP propagation](examples/propagation) carries W3C trace context between services.

`AddSource` provides one caller location, not a full stack trace. No `request_id`
is required; applications can attach one as context metadata when useful.

## Collection and delivery

Applications write JSON logs and completed spans to their configured output.
The separate [`slogx-collector`](https://github.com/rah-0/slogx-collector) executable
handles journaling, backend conversion, and delivery; applications do not import it.

For trace conversion, keep the reserved `span` group at the record root and
preserve its metadata names and types. Group application attributes with
`slog.Group`; see the [span record contract](examples/tracing/README.md#record-format).
A logging call confirms neither delivery nor durable storage; durability begins
when the collector writes and syncs the record to its journal.

## Tests

Run the library and example tests from the repository root:

```sh
go test -count=1 -race -cover -covermode=atomic ./...
go vet ./...
```

## Support

If `slogx` has saved you time or helped you track down a tricky bug, consider
buying me a coffee. Your support helps me keep improving the library.

[![Buy Me A Coffee](https://cdn.buymeacoffee.com/buttons/default-orange.png)](https://www.buymeacoffee.com/rah.0)

Licensed under the [MIT License](LICENSE).
