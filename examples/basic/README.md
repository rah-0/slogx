# Basic logging

Run from the repository root:

```sh
go run ./examples/basic
```

`slogx.New` returns a native `*slog.Logger`. This example binds a shared `service`
with `With`, puts request fields under `request` with `WithGroup`, and nests HTTP
fields with `slog.Group`. Fields bound before `WithGroup`, such as `service`,
remain at the record root.

`AddSource` adds the logging call's `function`, `file`, and `line` under `source`.
It reports one caller location, not a stack trace.

The example explicitly writes JSON to stdout. With an omitted writer, `New`
uses stderr and native slog timestamps. `NewDefault` uses stdout and UTC
microsecond timestamps. `SetDefault` installs `NewDefault` as the process-wide
logger for calls such as `slog.Info`.
