// Command nethttp is a minimal service using baton with only net/http.
//
//	go run ./examples/nethttp
//	curl -i localhost:8080/            # an ID is created and echoed
//	curl -i -H 'X-Request-ID: abc-1' localhost:8080/   # the ID is kept
//
// Each request to / calls /downstream on the same server through the baton
// transport, so both log lines share one request_id.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/agarwalvivek29/baton"
)

func main() {
	logger := slog.New(baton.LogHandler(slog.NewJSONHandler(os.Stdout, nil)))
	client := &http.Client{Transport: baton.Transport(nil, baton.WithOnMissing(func(r *http.Request) {
		logger.WarnContext(r.Context(), "outbound call has no request ID", "url", r.URL.String())
	}))}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "handling request", "path", r.URL.Path)
		req, err := http.NewRequestWithContext(r.Context(), "GET", "http://localhost:8080/downstream", nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			logger.ErrorContext(r.Context(), "downstream failed", "err", err)
			http.Error(w, "downstream failed", 502)
			return
		}
		resp.Body.Close()
		w.Write([]byte("ok " + baton.FromContext(r.Context()) + "\n"))
	})
	mux.HandleFunc("/downstream", func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "downstream got it")
	})

	logger.Info("listening", "addr", ":8080")
	if err := http.ListenAndServe(":8080", baton.Middleware(mux)); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
