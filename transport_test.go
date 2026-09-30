package baton

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// capture is a RoundTripper that records the request it was given.
type capture struct{ got *http.Request }

func (c *capture) RoundTrip(r *http.Request) (*http.Response, error) {
	c.got = r
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
}

func send(t *testing.T, rt http.RoundTripper, req *http.Request) {
	t.Helper()
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestTransportForwardsFromContext(t *testing.T) {
	c := &capture{}
	req := httptest.NewRequest("GET", "http://b/", nil).WithContext(NewContext(context.Background(), "id-1"))
	req.RequestURI = ""
	send(t, Transport(c), req)
	if got := c.got.Header.Get("X-Request-ID"); got != "id-1" {
		t.Fatalf("outbound id = %q, want id-1", got)
	}
	if req.Header.Get("X-Request-ID") != "" {
		t.Error("caller's request was mutated")
	}
	if c.got == req {
		t.Error("base got the caller's request, want a clone")
	}
}

func TestTransportKeepsExplicitHeader(t *testing.T) {
	c := &capture{}
	req, _ := http.NewRequestWithContext(NewContext(context.Background(), "from-ctx"), "GET", "http://b/", nil)
	req.Header.Set("X-Request-ID", "explicit")
	send(t, Transport(c), req)
	if got := c.got.Header.Get("X-Request-ID"); got != "explicit" {
		t.Fatalf("outbound id = %q, want explicit header kept", got)
	}
}

func TestTransportMissingIDHook(t *testing.T) {
	c := &capture{}
	var fired *http.Request
	rt := Transport(c, WithOnMissing(func(r *http.Request) { fired = r }))
	req, _ := http.NewRequest("GET", "http://b/", nil) // context.Background()
	send(t, rt, req)
	if fired == nil {
		t.Fatal("OnMissing hook did not fire")
	}
	if got := c.got.Header.Get("X-Request-ID"); got != "" {
		t.Errorf("Transport minted an id %q; it must not", got)
	}

	fired = nil
	req2, _ := http.NewRequestWithContext(NewContext(context.Background(), "x"), "GET", "http://b/", nil)
	send(t, rt, req2)
	if fired != nil {
		t.Error("hook fired although ctx had an ID")
	}
	fired = nil
	req3, _ := http.NewRequest("GET", "http://b/", nil)
	req3.Header.Set("X-Request-ID", "explicit")
	send(t, rt, req3)
	if fired != nil {
		t.Error("hook fired although header was set explicitly")
	}
}

func TestTransportForwardAllowlist(t *testing.T) {
	var outbound http.Header
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outbound = r.Header.Clone()
	}))
	defer b.Close()
	client := &http.Client{Transport: Transport(nil)}

	a := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, _ := http.NewRequestWithContext(r.Context(), "GET", b.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		resp.Body.Close()
	}), WithForward([]string{"X-Tenant"}))

	in := httptest.NewRequest("GET", "/", nil)
	in.Header.Set("X-Request-ID", "rid")
	in.Header.Set("X-Tenant", "t1")
	in.Header.Set("X-User-Email", "someone@example.com")
	a.ServeHTTP(httptest.NewRecorder(), in)

	if got := outbound.Get("X-Request-ID"); got != "rid" {
		t.Errorf("X-Request-ID = %q", got)
	}
	if got := outbound.Get("X-Tenant"); got != "t1" {
		t.Errorf("X-Tenant = %q, want forwarded", got)
	}
	if got := outbound.Get("X-User-Email"); got != "" {
		t.Errorf("X-User-Email forwarded (%q); only allowlisted headers may be", got)
	}
}

func TestTransportNilBase(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Request-ID")
	}))
	defer srv.Close()
	req, _ := http.NewRequestWithContext(NewContext(context.Background(), "nil-base"), "GET", srv.URL, nil)
	resp, err := (&http.Client{Transport: Transport(nil)}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != "nil-base" {
		t.Errorf("server saw %q", got)
	}
}

func TestTransportBaggage(t *testing.T) {
	ctx := NewContext(context.Background(), "rid")
	cases := []struct {
		name, in, want string
	}{
		{"no baggage", "", "request_id=rid"},
		{"merge", "tenant=t1", "tenant=t1,request_id=rid"},
		{"already set", "request_id=other,tenant=t1", "request_id=other,tenant=t1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &capture{}
			req, _ := http.NewRequestWithContext(ctx, "GET", "http://b/", nil)
			if tc.in != "" {
				req.Header.Set("Baggage", tc.in)
			}
			send(t, Transport(c, WithBaggage("")), req)
			if got := c.got.Header.Get("Baggage"); got != tc.want {
				t.Errorf("baggage = %q, want %q", got, tc.want)
			}
			if req.Header.Get("Baggage") != tc.in {
				t.Error("caller's baggage header was mutated")
			}
		})
	}

	c := &capture{}
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://b/", nil)
	send(t, Transport(c), req)
	if got := c.got.Header.Get("Baggage"); got != "" {
		t.Errorf("baggage %q set while WithBaggage is off", got)
	}
}
