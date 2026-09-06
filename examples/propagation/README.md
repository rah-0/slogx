# W3C HTTP propagation

Run from the repository root:

```sh
go run ./examples/propagation
```

[main.go](main.go) simulates an incoming HTTP request and a downstream service on
a local `httptest` server. No external service is required. The program extracts
the incoming parent, starts a server span, then starts a client span and injects
its context into the outgoing request. The downstream server extracts that
context before starting its own span. All requests are made by the example's
standard `net/http` code; slogx's propagation helpers only read and write headers.

All three spans share the incoming trace ID. Their parents form
`upstream → handle request → fetch inventory → serve inventory`. The incoming
and forwarded `traceparent` and `tracestate` values are logged so the boundary is
visible. [main_test.go](main_test.go) verifies this chain, headers, trace state,
log correlation, and request/response error handling without fixed random IDs or
timestamps.

Propagation follows the [23 November 2021 W3C Trace Context Level 1 Recommendation](https://www.w3.org/TR/2021/REC-trace-context-1-20211123/).
The parser validates versions, identifiers, and flags; unknown future-version
suffixes are not parsed. Outgoing `traceparent` uses version `00` and only the
sampled flag. Invalid, missing, or repeated incoming parents clear an inherited
identity, so `StartSpan` creates a new trace. Invalid `tracestate` is discarded
independently of a valid parent.

`tracestate` is opaque vendor metadata, separate from application attributes. Its
ordered validation accepts up to 32 comma-separated members, including empty
members, without a 512-byte total-length cap. Repeated HTTP fields are combined
in their value-slice order.

Use `http.Header.Set` or `Add`, or canonical keys such as `Traceparent` and
`Tracestate` in map literals. Injection replaces existing trace headers; invalid
context removes both headers, and invalid or empty trace state is omitted.

`ParseTraceparent` parses a field without trimming whitespace. It returns exported
sentinels for invalid length, version, trace ID, span ID, or flags; use `errors.Is`
to distinguish them. `SpanContext.Traceparent` formats a field, returning an empty
string for invalid IDs. `ContextWithSpanContext` and `SpanContextFromContext` also
carry identities across non-HTTP boundaries.

See [tracing](../tracing) for span lifecycle, recording conditions, and source
information.
