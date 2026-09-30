package baton

import "net/http"

// Transport wraps base so every outbound request carries the request ID from
// its context, plus any headers Middleware stored through WithForward. If base
// is nil, http.DefaultTransport is used.
//
// A header already set on the outbound request is left alone. The caller's
// request is never modified; Transport sends a clone. With WithBaggage, the ID
// is also merged into the W3C "baggage" header.
//
// If the request has no ID at all (typically because it was made with
// context.Background()), it is sent without one and the WithOnMissing hook
// runs. Transport never mints a new ID: that would split one request into two.
func Transport(base http.RoundTripper, opts ...Option) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &transport{base: base, c: newConfig(opts)}
}

type transport struct {
	base http.RoundTripper
	c    *config
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var ctxID string
	var fwd http.Header
	if s := scopeFrom(req.Context()); s != nil {
		ctxID, fwd = s.id, s.forward
	}

	// The ID this request goes out with: an explicit header wins over ctx.
	id := req.Header.Get(t.c.header)
	explicit := id != ""
	if !explicit {
		id = ctxID
	}
	if id == "" {
		t.c.onMissing(req)
	}

	setHeader := !explicit && id != ""
	setBag := t.c.baggage != "" && valid(id) && !hasBaggageMember(req.Header, t.c.baggage)
	if !setHeader && !setBag && len(fwd) == 0 {
		return t.base.RoundTrip(req)
	}

	out := req.Clone(req.Context())
	if setHeader {
		out.Header.Set(t.c.header, id)
	}
	for h, vs := range fwd {
		if _, ok := out.Header[h]; !ok {
			out.Header[h] = append([]string(nil), vs...)
		}
	}
	if setBag {
		setBaggage(out.Header, t.c.baggage, id)
	}
	return t.base.RoundTrip(out)
}
