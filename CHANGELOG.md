# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `Middleware`: reads and validates the inbound request ID (1–128 chars of `[A-Za-z0-9._-]`), or creates one. Stores it in the context, sets it on `r.Header`, and echoes it on the response.
- `Transport`: forwards the ID and allowlisted headers on outbound calls from the request context. Never modifies the caller's request and never mints an ID.
- `LogHandler`: adds a top-level `request_id` to `log/slog` records, also inside `WithGroup`.
- `FromContext`, `NewContext`, `NewID`.
- Options: `WithHeader`, `WithGenerator`, `WithForward`, `WithLogKey`, `WithOnMissing`, `WithBaggage` (opt-in W3C Baggage).
- Runnable examples and `examples/nethttp`.
