# Contributing to baton

Thanks for helping. `baton` is deliberately small, so the most useful contributions are bug reports, tests for edge cases, and docs. Please read the design rules before proposing a feature.

## Design rules

These are what keep `baton` easy to trust and audit. A PR that breaks one needs a very good reason:

1. **Standard library only.** No third-party imports in the root module, including in tests (use `testing` and `net/http/httptest`). Framework examples (chi, gin, ...) go in their own module under `examples/` with their own `go.mod`.
2. **HTTP in, HTTP out.** Other transports (Kafka, SQS, gRPC) are on the roadmap, not in v0.1.
3. **Small API.** New exported names need a clear use case. Prefer an `Option` over a new function.
4. **Create IDs only at the edge.** `Transport` must never mint an ID. That's the "one request becomes two traces" bug.
5. **Forward nothing by default.** Only the request ID and explicitly allowlisted headers leave the service.
6. **Validate anything from the client.** Inbound values that end up in logs go through the same validation as the ID.
7. **No metrics.** The ID must never become a metric label.
8. **Go 1.22 is the floor.** Don't use APIs newer than Go 1.22.

## Development

```bash
git clone https://github.com/agarwalvivek29/baton
cd baton
go test -race ./...
```

Before opening a PR, all of these must pass:

```bash
gofmt -l .            # must print nothing
go vet ./...
go test -race -count=1 ./...
```

To check the Go 1.22 floor (works on any OS with Docker):

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.22 go test -race ./...
```

## Tests

- Every behaviour change needs a test. Bug fixes need a test that fails without the fix.
- Keep tests table-driven where it helps, and hermetic: use `httptest` servers and no network.
- If you change public behaviour, update the runnable examples in `example_test.go` (they appear on pkg.go.dev) and the README in the same PR.

## Commits and pull requests

- One logical change per PR. Small PRs get reviewed faster.
- Write commit subjects in the imperative mood (`Add WithLogKey option`), 72 characters or fewer, with a body explaining *why* when it isn't obvious.
- Describe what you changed, why, and how you tested it in the PR description.

## Reporting bugs

Open an issue with:

- the Go version and `baton` version (or commit)
- a minimal reproduction, ideally a failing test
- what you expected and what happened

For a **security issue**, don't open a public issue. Follow [SECURITY.md](SECURITY.md).

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By taking part, you agree to uphold it.

## License

By contributing, you agree that your contributions are licensed under the [Apache License 2.0](LICENSE), the license of this project.
