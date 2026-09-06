# Examples

Each directory is a standalone Go program with a companion `main_test.go` and
notes about its behavior. Start with `basic`, then choose the case you need.
All examples use the root module, the Go standard library, and `slogx`.

| Example | What it demonstrates |
| --- | --- |
| [basic](basic) | Native logger setup, bound fields, nested groups, and caller source. |
| [formats](formats) | Text, JSON, colored text, systemd priorities, and timestamp layouts. |
| [levels](levels) | Changing the enabled log level with `slog.LevelVar`. |
| [errors](errors) | Three function layers carrying structured error attributes without context. |
| [context](context) | Explicit context metadata flowing through functions and remaining isolated. |
| [redaction](redaction) | Filtering nested attributes with native `ReplaceAttr`. |
| [identifiers](identifiers) | Generating trace and span IDs without starting operations. |
| [tracing](tracing) | Three parent/child spans, correlated logs, source, status, and error events. |
| [propagation](propagation) | W3C trace context across a local HTTP client and server. |
| [decoding](decoding) | Reading JSON logs while preserving attribute order, duplicates, and raw values. |

## Run and test

Run these commands from the repository root, replacing `basic` with another directory:

```sh
go run ./examples/basic
go test -count=1 -race -cover -covermode=atomic ./examples/basic
```

To test every example:

```sh
go test -count=1 -race -cover -covermode=atomic ./examples/...
```

Examples run without credentials, input files, or an external backend. The
propagation example starts an HTTP server on a local ephemeral port. The errors,
context, and tracing examples include deliberate failures to show how they are
logged; these are part of the demonstration.
