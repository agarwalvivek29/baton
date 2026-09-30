package baton

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
)

// DefaultHeader is the header that carries the request ID unless WithHeader
// says otherwise.
const DefaultHeader = "X-Request-ID"

// DefaultLogKey is the slog attribute name LogHandler uses unless WithLogKey
// says otherwise.
const DefaultLogKey = "request_id"

// maxIDLen is the longest inbound ID Middleware accepts as-is.
const maxIDLen = 128

type config struct {
	header    string
	generate  func() string
	forward   []string
	logKey    string
	onMissing func(*http.Request)
	baggage   string // W3C baggage member key; "" means off
}

// Option configures Middleware, Transport and LogHandler. Every option is
// accepted by all three; each piece reads only the options that concern it.
type Option func(*config)

// WithHeader sets the header that carries the request ID. The default is
// "X-Request-ID". Pass the same value to Middleware and Transport.
func WithHeader(name string) Option {
	return func(c *config) {
		if name != "" {
			c.header = http.CanonicalHeaderKey(name)
		}
	}
}

// WithGenerator sets how Middleware creates an ID when the inbound request has
// none (or an invalid one). The default is 16 random bytes, hex encoded.
func WithGenerator(fn func() string) Option {
	return func(c *config) {
		if fn != nil {
			c.generate = fn
		}
	}
}

// WithForward names extra inbound headers that Middleware stores in the
// context and Transport forwards on outbound calls. Headers not listed are
// never forwarded. Keep user identifiers out of this list; log them instead.
func WithForward(headers []string) Option {
	return func(c *config) {
		for _, h := range headers {
			if h != "" {
				c.forward = append(c.forward, http.CanonicalHeaderKey(h))
			}
		}
	}
}

// WithLogKey sets the attribute name LogHandler adds. The default is
// "request_id".
func WithLogKey(key string) Option {
	return func(c *config) {
		if key != "" {
			c.logKey = key
		}
	}
}

// WithOnMissing sets a hook that Transport calls when an outbound request has
// no request ID, neither in its context nor set explicitly. The usual cause is
// a call made with context.Background(). The default hook logs at debug level
// through slog.Default().
func WithOnMissing(fn func(*http.Request)) Option {
	return func(c *config) {
		if fn != nil {
			c.onMissing = fn
		}
	}
}

// WithBaggage turns on W3C Baggage (https://www.w3.org/TR/baggage/) with the
// member name key, or "request_id" if key is "". Transport then adds
// key=<id> to the outbound "baggage" header, merging with any members already
// there, and Middleware falls back to that member when the ID header is
// missing or invalid. Services instrumented with an OpenTelemetry SDK can then
// read the ID from baggage. It is off by default because baggage is often
// propagated to third parties.
func WithBaggage(key string) Option {
	return func(c *config) {
		if key == "" {
			key = DefaultLogKey
		}
		c.baggage = key
	}
}

func newConfig(opts []Option) *config {
	c := &config{
		header:    DefaultHeader,
		generate:  NewID,
		logKey:    DefaultLogKey,
		onMissing: logMissing,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func logMissing(r *http.Request) {
	slog.Default().DebugContext(r.Context(), "baton: outbound request has no request ID",
		"method", r.Method, "host", r.URL.Host, "path", r.URL.Path)
}

// NewID returns a new request ID: 16 random bytes from crypto/rand, hex
// encoded (32 characters).
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("baton: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// valid reports whether id is safe to accept from a client and write to logs:
// 1 to 128 characters from [A-Za-z0-9._-].
func valid(id string) bool {
	if id == "" || len(id) > maxIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

type ctxKey struct{}

type scope struct {
	id      string
	forward http.Header // allowlisted inbound headers; nil if none
}

// FromContext returns the request ID stored in ctx, or "" if there is none.
func FromContext(ctx context.Context) string {
	if s, ok := ctx.Value(ctxKey{}).(*scope); ok {
		return s.id
	}
	return ""
}

// NewContext returns a copy of ctx that carries id. Use it to start a scope
// outside an HTTP request, such as a background job or a test. Forwarded
// headers already in ctx are kept.
func NewContext(ctx context.Context, id string) context.Context {
	s := &scope{id: id}
	if old, ok := ctx.Value(ctxKey{}).(*scope); ok {
		s.forward = old.forward
	}
	return context.WithValue(ctx, ctxKey{}, s)
}

func scopeFrom(ctx context.Context) *scope {
	s, _ := ctx.Value(ctxKey{}).(*scope)
	return s
}
