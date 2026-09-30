// Package baton passes a request ID across HTTP hops so application logs can be
// joined to traces produced by eBPF instrumentation (Beyla / OBI), which the
// application itself never sees.
//
// It provides three pieces: an inbound middleware that reads or creates the ID,
// an http.RoundTripper that forwards it on outbound calls, and a log/slog
// handler that stamps it on every log line.
package baton
