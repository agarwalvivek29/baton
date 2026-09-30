package baton

import (
	"context"
	"net/http"
)

// Middleware reads the request ID from the inbound request, or creates one if
// it is missing or invalid, and stores it in the request context.
//
// An inbound ID is accepted only if it is 1 to 128 characters from
// [A-Za-z0-9._-]; anything else is replaced, so clients can't inject log
// content through the header. The ID is also set on r.Header, so handler code
// that reads the header sees the same value, and echoed on the response.
//
// With WithBaggage, a missing or invalid header falls back to the ID in the
// inbound W3C baggage. Headers named with WithForward are stored in the
// context too, for Transport to forward.
func Middleware(next http.Handler, opts ...Option) http.Handler {
	c := newConfig(opts)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(c.header)
		if !valid(id) && c.baggage != "" {
			id = baggageValue(r.Header, c.baggage)
		}
		if !valid(id) {
			id = c.generate()
		}

		s := &scope{id: id}
		for _, h := range c.forward {
			if h == c.header {
				continue
			}
			if vs := r.Header.Values(h); len(vs) > 0 {
				if s.forward == nil {
					s.forward = make(http.Header, len(c.forward))
				}
				s.forward[h] = append([]string(nil), vs...)
			}
		}

		r.Header.Set(c.header, id)
		w.Header().Set(c.header, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, s)))
	})
}
