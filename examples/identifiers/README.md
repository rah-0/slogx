# Trace and span identifiers

Run from the repository root:

```sh
go run ./examples/identifiers
```

[main.go](main.go) writes one ordinary JSON log with independently generated IDs.
`NewTraceID` produces 32 lowercase hexadecimal characters from 16 random bytes;
`NewSpanID` produces 16 characters from 8 bytes. Both preserve leading zeros and
retry an all-zero result.

The formats follow [W3C Trace Context Level 1](https://www.w3.org/TR/2021/REC-trace-context-1-20211123/#version-format);
the span ID has the format of the header's `parent-id` field. Both generators use
`crypto/rand`. Since [Go 1.24](https://go.dev/doc/go1.24#cryptorand), `rand.Read`
fills its buffer or terminates the process on a read error, so the generators do
not return errors.

Adding these fields does not start a trace or span. The
[tracing example](../tracing) creates operations with shared trace IDs and distinct
span IDs.

[main_test.go](main_test.go) verifies both ID formats and that the output remains
an ordinary log record.
