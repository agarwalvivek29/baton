package baton_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/agarwalvivek29/baton"
)

// A service wires baton in three places: inbound, outbound and logs.
func Example() {
	logger := slog.New(baton.LogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: dropTime, // stable output for this example
	})))
	client := &http.Client{Transport: baton.Transport(nil)}

	downstream := httptest.NewServer(baton.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "downstream saw it")
	})))
	defer downstream.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "edge saw it")
		req, _ := http.NewRequestWithContext(r.Context(), "GET", downstream.URL, nil)
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
		}
	})
	edge := httptest.NewServer(baton.Middleware(mux))
	defer edge.Close()

	req, _ := http.NewRequest("GET", edge.URL, nil)
	req.Header.Set("X-Request-ID", "demo-1")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	fmt.Println("echoed:", resp.Header.Get("X-Request-ID"))
	// Output:
	// {"level":"INFO","msg":"edge saw it","request_id":"demo-1"}
	// {"level":"INFO","msg":"downstream saw it","request_id":"demo-1"}
	// echoed: demo-1
}

func dropTime(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && len(groups) == 0 {
		return slog.Attr{}
	}
	return a
}

// FromContext reads the ID anywhere the request context reaches.
func ExampleFromContext() {
	h := baton.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(baton.FromContext(r.Context()))
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	h.ServeHTTP(httptest.NewRecorder(), req)
	// Output: abc-123
}

// NewContext starts a scope outside HTTP, e.g. in a background job.
func ExampleNewContext() {
	ctx := baton.NewContext(context.Background(), "job-42")
	fmt.Println(baton.FromContext(ctx))
	// Output: job-42
}

// WithOnMissing makes a dropped context visible.
func ExampleWithOnMissing() {
	rt := baton.Transport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}), baton.WithOnMissing(func(r *http.Request) {
		fmt.Println("no request ID on call to", r.URL.Host)
	}))
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://payments/charge", nil)
	resp, _ := rt.RoundTrip(req)
	resp.Body.Close()
	// Output: no request ID on call to payments
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
