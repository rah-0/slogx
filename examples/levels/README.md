# Runtime log levels

Run from the repository root:

```sh
go run ./examples/levels
```

Pass a `*slog.LevelVar` through `Options.Level` to change the minimum enabled
level without rebuilding the logger. The example starts at `INFO`, enables
`DEBUG`, then raises the threshold to `WARN`.

Only three records are emitted: the initial info, the enabled debug message,
and the warning. The debug call before its level is enabled and the info call
after the threshold is raised are suppressed.
