# Output formats and timestamps

Run from the repository root:

```sh
go run ./examples/formats
```

The example writes the same warning in `Text`, `JSON`, `TextColored`, and
`Systemd` formats. `TimeLayout` uses Go's layout syntax and formats timestamps
in UTC; the example uses `2006/01/02 15:04:05`.

`TextColored` colors standard level names with ANSI escapes even in files and
pipes; it does not detect terminals. Custom levels remain uncolored. Use
`JSON` when piping records to a JSON collector.

`Systemd` writes uncolored text with a priority prefix. Outside a journal-connected
stream the prefix is visible, such as `<4>` for the example's warning.

| slog level range | Journal priority |
| --- | --- |
| Below `INFO` | `7` (debug) |
| `INFO` to below `WARN` | `6` (info) |
| `WARN` to below `ERROR` | `4` (warning) |
| `ERROR` and above | `3` (error) |

For a service that uses `Systemd`, configure its unit to capture output and
interpret these prefixes, as described by
[systemd.exec](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#SyslogLevelPrefix=):

```ini
[Service]
StandardOutput=journal
StandardError=journal
SyslogLevelPrefix=yes
```

Keep the native `level` key and value when using `ReplaceAttr` with `TextColored`
or `Systemd`; their color and priority are derived from the rendered level.
