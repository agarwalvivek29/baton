package baton

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve runs one request through Middleware and returns the ID the handler
// saw in its context, the ID on r.Header, and the response.
func serve(t *testing.T, req *http.Request, opts ...Option) (ctxID, hdrID string, resp *http.Response) {
	t.Helper()
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = FromContext(r.Context())
		hdrID = r.Header.Get(newConfig(opts).header)
	}), opts...)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return ctxID, hdrID, rec.Result()
}

func TestMiddlewareGeneratesWhenMissing(t *testing.T) {
	id, hdr, resp := serve(t, httptest.NewRequest("GET", "/", nil))
	if len(id) != 32 || !valid(id) {
		t.Fatalf("generated id = %q, want 32 hex chars", id)
	}
	if hdr != id {
		t.Errorf("r.Header id = %q, want %q", hdr, id)
	}
	if got := resp.Header.Get("X-Request-ID"); got != id {
		t.Errorf("response echo = %q, want %q", got, id)
	}
}

func TestMiddlewareKeepsValidID(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "abc-123_DEF.9")
	id, _, resp := serve(t, req)
	if id != "abc-123_DEF.9" {
		t.Fatalf("id = %q, want inbound value kept", id)
	}
	if got := resp.Header.Get("X-Request-ID"); got != id {
		t.Errorf("response echo = %q", got)
	}
}

func TestMiddlewareReplacesInvalidID(t *testing.T) {
	for name, bad := range map[string]string{
		"too long":   strings.Repeat("a", 129),
		"space":      "abc def",
		"quote":      `abc"def`,
		"json break": `x","level":"ERROR`,
		"CRLF":       "abc\r\nX-Evil: 1",
		"newline":    "abc\nfake log line",
		"unicode":    "abcé",
		"slash":      "../../etc",
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header["X-Request-Id"] = []string{bad}
			id, hdr, _ := serve(t, req)
			if id == bad || !valid(id) {
				t.Fatalf("id = %q, want a fresh valid ID", id)
			}
			if hdr != id {
				t.Errorf("r.Header still carries %q, want %q", hdr, id)
			}
		})
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", strings.Repeat("a", 128))
	if id, _, _ := serve(t, req); len(id) != 128 {
		t.Errorf("128-char id was replaced")
	}
}

func TestMiddlewareOptions(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	id, hdr, resp := serve(t, req,
		WithHeader("x-correlation-id"),
		WithGenerator(func() string { return "fixed" }))
	if id != "fixed" || hdr != "fixed" {
		t.Fatalf("id = %q, hdr = %q, want custom generator used", id, hdr)
	}
	if got := resp.Header.Get("X-Correlation-ID"); got != "fixed" {
		t.Errorf("echo on custom header = %q", got)
	}
	if got := resp.Header.Get("X-Request-ID"); got != "" {
		t.Errorf("default header should be unused, got %q", got)
	}
}

func TestMiddlewareStoresForwardAllowlist(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Tenant", "t1")
	req.Header.Set("X-User-Email", "someone@example.com") // not allowlisted
	var fwd http.Header
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fwd = scopeFrom(r.Context()).forward
	}), WithForward([]string{"x-tenant", "X-Absent"}))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got := fwd.Get("X-Tenant"); got != "t1" {
		t.Errorf("forward X-Tenant = %q", got)
	}
	if _, ok := fwd["X-User-Email"]; ok {
		t.Error("non-allowlisted header stored")
	}
	if _, ok := fwd["X-Absent"]; ok {
		t.Error("absent header stored")
	}
}

func TestMiddlewareBaggageFallback(t *testing.T) {
	req := func(hdr ...string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		for i := 0; i < len(hdr); i += 2 {
			r.Header.Set(hdr[i], hdr[i+1])
		}
		return r
	}
	const bag = "tenant=t1, request_id=from-bag;prop=1"

	if id, _, _ := serve(t, req("Baggage", bag), WithBaggage("")); id != "from-bag" {
		t.Errorf("id = %q, want baggage fallback", id)
	}
	if id, _, _ := serve(t, req("Baggage", bag)); id == "from-bag" {
		t.Error("baggage read while WithBaggage is off")
	}
	if id, _, _ := serve(t, req("Baggage", bag, "X-Request-ID", "from-header"), WithBaggage("")); id != "from-header" {
		t.Errorf("id = %q, header must win over baggage", id)
	}
	if id, _, _ := serve(t, req("Baggage", "request_id=a%0Ab"), WithBaggage("")); id == "a\nb" || !valid(id) {
		t.Errorf("invalid baggage id accepted: %q", id)
	}
}
