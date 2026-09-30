package baton

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// syncBuffer is a bytes.Buffer safe for concurrent log writes.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// TestEndToEnd: A receives a request with no ID, calls B through Transport
// (once on the request goroutine, once from a spawned goroutine), and both
// services log. Every log line from both must carry the same ID.
func TestEndToEnd(t *testing.T) {
	var logs syncBuffer
	logger := slog.New(LogHandler(slog.NewJSONHandler(&logs, nil)))

	b := httptest.NewServer(Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "b handled", "svc", "b")
	})))
	defer b.Close()

	client := &http.Client{Transport: Transport(nil)}
	call := func(r *http.Request) {
		req, _ := http.NewRequestWithContext(r.Context(), "GET", b.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		resp.Body.Close()
	}
	a := httptest.NewServer(Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "a handled", "svc", "a")
		call(r)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); call(r) }()
		wg.Wait()
	})))
	defer a.Close()

	resp, err := http.Get(a.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	id := resp.Header.Get("X-Request-ID")
	if !valid(id) {
		t.Fatalf("edge did not echo a valid ID: %q", id)
	}

	lines := strings.Split(strings.TrimSpace(logs.b.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 log lines, got %d: %s", len(lines), logs.b.String())
	}
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		if m["request_id"] != id {
			t.Errorf("%s logged request_id %v, want %s", m["svc"], m["request_id"], id)
		}
	}
}
