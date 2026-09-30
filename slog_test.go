package baton

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func jsonLogger(buf *bytes.Buffer, opts ...Option) *slog.Logger {
	return slog.New(LogHandler(slog.NewJSONHandler(buf, nil), opts...))
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("bad json %q: %v", buf.String(), err)
	}
	buf.Reset()
	return m
}

func TestLogHandlerAddsID(t *testing.T) {
	var buf bytes.Buffer
	log := jsonLogger(&buf)
	log.InfoContext(NewContext(context.Background(), "rid"), "hi")
	if got := decode(t, &buf)["request_id"]; got != "rid" {
		t.Errorf("request_id = %v", got)
	}

	log.InfoContext(context.Background(), "hi")
	if _, ok := decode(t, &buf)["request_id"]; ok {
		t.Error("request_id present without an ID in ctx")
	}
}

func TestLogHandlerLogKey(t *testing.T) {
	var buf bytes.Buffer
	jsonLogger(&buf, WithLogKey("rid")).InfoContext(NewContext(context.Background(), "x"), "hi")
	if got := decode(t, &buf)["rid"]; got != "x" {
		t.Errorf("rid = %v", got)
	}
}

func TestLogHandlerWithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	ctx := NewContext(context.Background(), "rid")

	jsonLogger(&buf).With("svc", "orders").InfoContext(ctx, "hi")
	m := decode(t, &buf)
	if m["request_id"] != "rid" || m["svc"] != "orders" {
		t.Errorf("WithAttrs: %v", m)
	}

	jsonLogger(&buf).With("svc", "orders").WithGroup("req").With("path", "/x").InfoContext(ctx, "hi", "n", 1)
	m = decode(t, &buf)
	if m["request_id"] != "rid" {
		t.Errorf("WithGroup: request_id not top-level: %v", m)
	}
	req, _ := m["req"].(map[string]any)
	if m["svc"] != "orders" || req["path"] != "/x" || req["n"] != float64(1) {
		t.Errorf("WithGroup: attrs lost or misplaced: %v", m)
	}
	if _, ok := req["request_id"]; ok {
		t.Errorf("request_id nested in group: %v", m)
	}

	jsonLogger(&buf).WithGroup("req").InfoContext(context.Background(), "hi")
	if _, ok := decode(t, &buf)["request_id"]; ok {
		t.Error("request_id present without an ID in ctx (group path)")
	}
}

func TestLogHandlerEnabled(t *testing.T) {
	h := LogHandler(slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Info enabled on a Warn handler")
	}
	if !h.WithGroup("g").Enabled(context.Background(), slog.LevelError) {
		t.Error("Error disabled after WithGroup")
	}
}
