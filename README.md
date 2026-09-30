# baton

**Pass the request ID to every hop, so your logs can find your eBPF traces.**

[![Go Reference](https://pkg.go.dev/badge/github.com/agarwalvivek29/baton.svg)](https://pkg.go.dev/github.com/agarwalvivek29/baton)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/go-%E2%89%A51.22-00ADD8)
![Dependencies](https://img.shields.io/badge/dependencies-stdlib%20only-brightgreen)

> **v0.1.0.** Pre-1.0: the API is small and meant to stay stable, but minor releases may still change it. See the [CHANGELOG](CHANGELOG.md).

---

- [The problem](#the-problem)
- [The idea](#the-idea)
- [Install](#install)
- [Quick start](#quick-start)
- [Two rules](#two-rules)
- [API](#api)
- [Options](#options)
- [W3C Baggage](#w3c-baggage)
- [Pairing it with an eBPF agent](#pairing-it-with-an-ebpf-agent)
- [Security](#security)
- [Sharp edges](#sharp-edges)
- [FAQ](#faq)
- [Roadmap](#roadmap)
- [Contributing](#contributing)

## The problem

eBPF tracing ([OpenTelemetry eBPF Instrumentation](https://github.com/open-telemetry/opentelemetry-ebpf-instrumentation), [Beyla](https://github.com/grafana/beyla)) gives you distributed traces with **zero code changes**. But the trace is created in the kernel, not in your app. Your service never learns its trace ID, so its logs can't carry one.

When an error log fires, it has no trace ID. You're back to matching timestamps across services.

## The idea

Don't push the trace ID into the app. Join on something both sides can see: a **request-ID header**.

```
  client ──► gateway ──► orders ──► payments
             │  X-Request-ID travels on every hop   (baton)
             │
  eBPF agent reads it in-kernel ──► span attribute
  app logs print it             ──► log field
                                    └──► search traces by it: log line ⇄ every trace of that request
```

There's one catch. eBPF can **read** the header, but it won't **forward** it. If a service doesn't pass it on its outbound calls, the chain breaks at that hop. `baton` is those few lines of code, written once and tested.

It also holds up when eBPF gets the trace wrong. When a request fans out through goroutines or worker pools, an eBPF agent can split it into several disconnected traces. The request ID is carried explicitly in `context.Context`, so every one of those traces still carries it. One search finds them all. See the [measured results](https://github.com/agarwalvivek29/baton-relay/blob/main/docs/goroutines.md).

## Install

```bash
go get github.com/agarwalvivek29/baton@v0.1.0
```

Go 1.22 or newer. **Standard library only**: `net/http`, `context`, `log/slog`, `crypto/rand`. There are no dependencies to audit.

## Quick start

```go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/agarwalvivek29/baton"
)

func main() {
	// Logs: every line logged with a request's ctx gets request_id=<id>.
	logger := slog.New(baton.LogHandler(slog.NewJSONHandler(os.Stdout, nil)))

	// Outbound: every call made with a request's ctx carries X-Request-ID.
	client := &http.Client{Transport: baton.Transport(nil)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /checkout", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger.InfoContext(ctx, "checkout started")

		req, _ := http.NewRequestWithContext(ctx, "POST", "http://orders:8081/orders", nil)
		resp, err := client.Do(req)
		if err != nil {
			logger.ErrorContext(ctx, "orders failed", "err", err)
			http.Error(w, "orders failed", http.StatusBadGateway)
			return
		}
		resp.Body.Close()
	})

	// Inbound: read the ID, or create one at the edge.
	http.ListenAndServe(":8080", baton.Middleware(mux))
}
```

```json
{"level":"INFO","msg":"checkout started","request_id":"4c08c1cf6109c69837ed33313f296513"}
```

A runnable version is in [`examples/nethttp`](examples/nethttp). A four-service system with an eBPF agent, Tempo, Loki and Grafana is in [**baton-relay**](https://github.com/agarwalvivek29/baton-relay).

## Two rules

**1. Pass the request's `ctx` to outbound calls.** Use `http.NewRequestWithContext(r.Context(), ...)`. That's how `Transport` finds the ID.

Go has no goroutine-local storage, so `ctx` is the only thing that carries the ID. When work moves to another goroutine, the `ctx` must go with it:

```go
go func() { call(ctx, url) }()            // capture it in the closure

jobs <- job{ctx: ctx, url: url}           // put it in the job for a worker pool

ctx := context.WithoutCancel(r.Context()) // for work that outlives the request:
                                          // keeps the ID, drops the cancellation
```

This is where the app beats eBPF. The agent has to guess which request a goroutine belongs to, and it can't for a worker pool started at boot. The app just hands the ID over.

**2. Log with the context variants.** Use `logger.InfoContext(ctx, ...)` and `logger.ErrorContext(ctx, ...)`. Plain `logger.Info(...)` has no context, so it can't add the ID.

## API

```go
func Middleware(next http.Handler, opts ...Option) http.Handler
func Transport(base http.RoundTripper, opts ...Option) http.RoundTripper
func LogHandler(h slog.Handler, opts ...Option) slog.Handler

func FromContext(ctx context.Context) string             // "" if absent
func NewContext(ctx context.Context, id string) context.Context
func NewID() string                                      // 16 random bytes, hex
```

| Piece | What it does |
|---|---|
| `Middleware` | Takes the ID from the inbound header if it's valid, or creates one. Stores it in the request context, sets it on `r.Header`, and echoes it on the response. |
| `Transport` | Clones each outbound request and adds the ID from its context. A header you set yourself wins. If there's no ID, the request goes out without one and the `WithOnMissing` hook runs. It **never** creates an ID mid-chain. `base == nil` means `http.DefaultTransport`. |
| `LogHandler` | Adds a **top-level** `request_id` attribute, also inside `WithGroup`, so log pipelines find it in the same place every time. |
| `FromContext` / `NewContext` | Read the ID anywhere, or start a scope outside HTTP (jobs, consumers, tests). |

`baton` works with any router built on `net/http` (the standard mux, chi, gorilla, and others). For gin or echo, wrap their `http.Handler` or adapt the middleware.

## Options

All three constructors take the same `Option` values. Each one reads only the options it needs.

| Option | Used by | Default |
|---|---|---|
| `WithHeader(name)` | Middleware, Transport | `X-Request-ID` |
| `WithGenerator(fn)` | Middleware | `NewID`: 16 bytes from `crypto/rand`, hex |
| `WithForward([]string{...})` | Middleware stores them, Transport sends them | none. Only listed headers are forwarded. |
| `WithBaggage(key)` | Middleware, Transport | off. `""` means the member name `request_id`. |
| `WithOnMissing(fn)` | Transport | logs a debug line through `slog.Default()` |
| `WithLogKey(key)` | LogHandler | `request_id` |

Pass the same `WithHeader` and `WithBaggage` to `Middleware` and `Transport`.

Make dropped IDs loud in your services:

```go
client := &http.Client{Transport: baton.Transport(nil, baton.WithOnMissing(func(r *http.Request) {
	logger.WarnContext(r.Context(), "outbound call has no request ID", "url", r.URL.String())
}))}
```

## W3C Baggage

`WithBaggage("")` makes `Transport` also add `request_id=<id>` to the outbound [`baggage`](https://www.w3.org/TR/baggage/) header. It merges with members already there and never overwrites an existing `request_id`. `Middleware` falls back to that member when the ID header is missing or invalid, and the value goes through the same validation.

This helps in mixed fleets, where services running an OpenTelemetry SDK can read the ID from baggage. Baggage is **off by default** because it is often propagated to third-party APIs. The header stays the primary carrier, because it's what the eBPF agent captures.

## Pairing it with an eBPF agent

The agent must record the header on its spans. With **OpenTelemetry eBPF Instrumentation (OBI) v0.13.0**, this config was tested on every hop of a four-service Go system:

```yaml
discovery:
  instrument:
    - open_ports: "8080-8083"
  skip_go_specific_tracers: true   # see note 1
ebpf:
  buffer_sizes:
    http: 8192                     # the default 0 captures nothing
  payload_extraction:
    http:
      enrichment:
        enabled: true
        policy:
          default_action: { headers: exclude, body: exclude }
        rules:
          - action: include
            type: headers
            scope: all              # request AND response, see note 2
            match: { patterns: ["X-Request-ID"], case_sensitive: false }
```

Then search Tempo with TraceQL:

```
{ span."http.request.header.x-request-id" = "<id>" || span."http.response.header.x-request-id" = "<id>" }
```

Notes from measuring it:

1. With OBI's Go-specific tracers, the header was only captured on the edge span. With the generic tracer, it was captured on every hop, for 80 of 80 requests.
2. At the edge, baton mints the ID, so the inbound request never carried it. The only place eBPF can see it on that span is the **response** header, which `Middleware` echoes.
3. OBI needs `/sys/kernel/tracing` mounted into its container. Without it, header capture silently doesn't happen.

The full, cited setup (compose file, Grafana log → trace link, and the numbers) is in [baton-relay](https://github.com/agarwalvivek29/baton-relay).

## Security

- **Inbound IDs are validated:** 1 to 128 characters from `[A-Za-z0-9._-]`. Anything else, such as newlines, quotes, JSON fragments or very long values, is replaced with a fresh ID. A client can't inject log lines or break your JSON logs through the header.
- **Only allowlisted headers are forwarded** (`WithForward`). Nothing else from the inbound request leaks downstream. Keep user identifiers log-only.
- **The caller's request is never modified.** `Transport` works on a clone, as the `RoundTripper` contract requires.

## Sharp edges

- **Dropped `context`.** A call made with `context.Background()` loses the ID at the next hop. `WithOnMissing` makes this visible.
- **A missing header mints a new ID.** If a hop drops the header, the next service starts a new ID and one request becomes two. That's why `Transport` never creates IDs, and why only the edge should.
- **Header leaks.** Forward only what's allowlisted.
- **Cardinality.** The ID is a span attribute and a log field, **never a metric label**. `baton` emits no metrics.

## FAQ

**Why not log the trace ID?** With eBPF instrumentation the app never sees it. If you can add an OpenTelemetry SDK to a service, do that and log the real trace ID. `baton` is for when you can't, or don't want to.

**Why not W3C Baggage only?** eBPF agents capture plain headers; they don't unpack baggage members into span attributes. Baggage is supported as an opt-in extra carrier.

**Does it work without an eBPF agent?** Yes. You still get one ID across every service's logs, which is useful on its own.

**Kafka, SQS, gRPC?** Not yet. v0.1 is HTTP in and HTTP out, on purpose.

## Roadmap

- [x] v0.1: middleware, transport, slog handler, opt-in W3C Baggage, tests
- [x] Example with `net/http`
- [x] Tag v0.1.0
- [ ] Examples with chi and gin (separate modules, so the root stays dependency-free)
- [ ] Maybe later: async hops (Kafka, SQS)

## Contributing

Issues and PRs are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[Apache-2.0](LICENSE). See [NOTICE](NOTICE).
