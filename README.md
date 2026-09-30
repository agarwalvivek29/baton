# baton

**Pass the request ID to every hop, so your logs can find your eBPF traces.**

> 🚧 Pre-release. The API may still change until v0.1.0 is tagged, ahead of MumbaiFOSS 2026 (Oct 31, IIT Bombay).

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

## Install

```bash
go get github.com/agarwalvivek29/baton
```

Go 1.22+. No dependencies outside the standard library.

## Usage

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

Two rules make it work:

1. **Pass the request's `ctx` to outbound calls:** `http.NewRequestWithContext(r.Context(), ...)`. That's how `Transport` finds the ID. This includes calls made from goroutines and worker pools: send the `ctx` along with the job.
2. **Log with the context variants:** `logger.InfoContext(ctx, ...)`, `logger.ErrorContext(ctx, ...)`. Plain `logger.Info(...)` has no context, so it can't add the ID.

A runnable service is in [`examples/nethttp`](examples/nethttp).

### What each piece does

- **`Middleware`**: takes the ID from the inbound header if it is valid (1–128 chars of `[A-Za-z0-9._-]`); otherwise it creates one. Anything else is replaced, so a client can't inject log lines or JSON through the header. The ID goes into the context, onto `r.Header`, and back on the response.
- **`Transport`**: clones the outbound request and adds the ID from its context. If you set the header yourself, yours wins. If there's no ID (usually a `context.Background()` call), the request goes out without one and the `WithOnMissing` hook runs. It never creates an ID mid-chain, because that turns one request into two.
- **`LogHandler`**: adds a top-level `request_id` attribute, including inside `WithGroup`, so log pipelines find it in the same place every time.
- **`NewContext(ctx, id)`**: starts a scope outside HTTP, for jobs, consumers and tests.

### Options

All three constructors take the same `Option` values. Each one reads only the options it needs.

| Option | Used by | Default |
|---|---|---|
| `WithHeader(name)` | Middleware, Transport | `X-Request-ID` |
| `WithGenerator(fn)` | Middleware | 16 random bytes from `crypto/rand`, hex |
| `WithForward([]string{...})` | Middleware stores them, Transport sends them | none. Only listed headers are forwarded. |
| `WithBaggage(key)` | Middleware, Transport | off. `""` means the member name `request_id`. |
| `WithOnMissing(fn)` | Transport | a debug-level `slog.Default()` line |
| `WithLogKey(key)` | LogHandler | `request_id` |

Pass the same `WithHeader` / `WithBaggage` to `Middleware` and `Transport`.

### W3C Baggage

With `WithBaggage("")`, `Transport` also adds `request_id=<id>` to the outbound [`baggage`](https://www.w3.org/TR/baggage/) header. It merges with members already there and never overwrites an existing `request_id`. `Middleware` falls back to that member when the ID header is missing or invalid. The fallback goes through the same validation as the header.

This helps in mixed fleets: services that run an OpenTelemetry SDK can read the ID from baggage and add it to their own spans. Baggage is **off by default** because it is often propagated to third-party APIs. The ID header stays the primary carrier, because it's the one the eBPF agent captures.

## Sharp edges this is designed around

- **Dropped `context`:** a call made with `context.Background()` loses the ID at the next hop. `baton` can log a warning when an outbound call has no ID.
- **Missing header mints a new ID:** one request becomes two traces. The middleware only creates an ID at the edge of your system.
- **Header leaks:** forward only allowlisted headers. Keep user identifiers log-only.
- **Cardinality:** the ID is a span attribute and a log field, **never** a metric label. `baton` emits no metrics.
- **When not to use it:** if you can add an OpenTelemetry SDK to a service, log the real trace ID instead.

## Roadmap

- [x] v0.1: middleware, transport, slog handler, opt-in W3C Baggage, tests
- [x] Example with `net/http`
- [ ] Examples with chi, gin (separate modules)
- [ ] Beyla/OBI config snippet for capturing the header on spans
- [ ] Later (maybe): async hops (Kafka, SQS)

## See it working

→ [**baton-relay**](https://github.com/agarwalvivek29/baton-relay): a small system of services traced by Beyla, with log-to-trace linking.

## License

[Apache-2.0](LICENSE)
