# baton

**Pass the request ID to every hop, so your logs can find your eBPF traces.**

> 🚧 Early stage. The API below is the design. Code is landing ahead of MumbaiFOSS 2026 (Oct 31, IIT Bombay).

## The problem

eBPF tracing ([Beyla](https://github.com/grafana/beyla) / [OpenTelemetry eBPF Instrumentation](https://github.com/open-telemetry/opentelemetry-ebpf-instrumentation)) gives you distributed traces with **zero code changes**. But the trace is created in the kernel, not in your app. Your service never learns its trace ID, so its logs can't carry one.

**eBPF sees every request your service handles, yet your service has no idea which trace it's in.**

## The idea

Don't push the trace ID into the app. Join on something both sides can see: a **request-ID header**.

- The eBPF agent reads HTTP in-kernel and records the header on the span.
- Your app prints the same header on every log line.
- Search your traces by that ID, and one log line opens its full trace.

There's one catch. eBPF can *read* the header, but it won't *forward* it. If a service doesn't pass it on its outbound calls, the chain breaks at that hop. `baton` is those few lines of code, written once.

```
  client ──► svc A ──► svc B ──► svc C
             │  X-Request-ID travels on every hop (baton)
             │
  eBPF agent reads it in-kernel ──► span attribute
  app logs print it             ──► log field
                                    └──► join: log line ⇄ full trace
```

## What it does (HTTP only)

| Piece | Job |
|---|---|
| **Middleware** | Reads `X-Request-ID` from the inbound request, or creates one. Stores it in `context.Context` and echoes it on the response. |
| **Client** | Wraps `http.RoundTripper` so every outbound call carries the ID from `ctx`. |
| **Log field** | Puts the ID on every log line through a `log/slog` handler. |

Standard library only: `net/http`, `context`, `log/slog`. Works with any router built on `net/http`.

## Planned API

```go
import "github.com/agarwalvivek29/baton"

// Inbound: read or create the request ID
mux := http.NewServeMux()
handler := baton.Middleware(mux)

// Outbound: forward it on every call
client := &http.Client{Transport: baton.Transport(http.DefaultTransport)}

// Logs: stamp it on every line
logger := slog.New(baton.LogHandler(slog.NewJSONHandler(os.Stdout, nil)))

// Anywhere: read it
id := baton.FromContext(ctx)
```

Options (planned):
- `baton.WithHeader("X-Correlation-ID")` to use a header name other than `X-Request-ID`
- `baton.WithGenerator(fn)` for a custom ID format
- `baton.WithForward([]string{...})` to choose headers that are forwarded vs. only logged

## Sharp edges this is designed around

- **Dropped `context`:** a call made with `context.Background()` loses the ID at the next hop. `baton` can log a warning when an outbound call has no ID.
- **Missing header mints a new ID:** one request becomes two traces. The middleware only creates an ID at the edge of your system.
- **Header leaks:** forward only allowlisted headers. Keep user identifiers log-only.
- **Cardinality:** the ID is a span attribute and a log field, **never** a metric label.
- **When not to use it:** if you can add an OpenTelemetry SDK to a service, log the real trace ID instead.

## Roadmap

- [ ] v0.1: middleware, transport, slog handler, tests
- [ ] Examples with `net/http`, chi, gin
- [ ] Beyla/OBI config snippet for capturing the header on spans
- [ ] Optional W3C Baggage emit
- [ ] Later (maybe): async hops (Kafka, SQS)

## See it working

→ [**baton-relay**](https://github.com/agarwalvivek29/baton-relay): a small system of services traced by Beyla, with log-to-trace linking.

## License

[Apache-2.0](LICENSE)
