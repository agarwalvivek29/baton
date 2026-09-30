package baton

import (
	"context"
	"log/slog"
)

// LogHandler wraps h so every record logged with a context that carries a
// request ID gets a top-level "request_id" attribute (see WithLogKey).
//
// Log with the context variants (InfoContext, ErrorContext, ...) and pass the
// request's context; the plain Info/Error methods have no context to read.
func LogHandler(h slog.Handler, opts ...Option) slog.Handler {
	c := newConfig(opts)
	return &logHandler{base: h, h: h, key: c.logKey}
}

type logHandler struct {
	base slog.Handler // the wrapped handler, before any WithAttrs/WithGroup
	h    slog.Handler // base with ops applied
	ops  []logOp      // WithAttrs/WithGroup calls, in order
	grp  bool         // true once WithGroup has been called
	key  string
}

type logOp struct {
	group string
	attrs []slog.Attr
}

func (l *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return l.h.Enabled(ctx, level)
}

func (l *logHandler) Handle(ctx context.Context, r slog.Record) error {
	id := FromContext(ctx)
	if id == "" {
		return l.h.Handle(ctx, r)
	}
	attr := slog.String(l.key, id)
	if !l.grp {
		// No open group: record attrs land at the top level.
		r = r.Clone()
		r.AddAttrs(attr)
		return l.h.Handle(ctx, r)
	}
	// Inside a group, record attrs would be nested. Rebuild the chain with the
	// ID added before the first group so it stays top-level.
	h := l.base.WithAttrs([]slog.Attr{attr})
	for _, op := range l.ops {
		if op.group != "" {
			h = h.WithGroup(op.group)
		} else {
			h = h.WithAttrs(op.attrs)
		}
	}
	return h.Handle(ctx, r)
}

func (l *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return l
	}
	return l.with(logOp{attrs: attrs}, l.h.WithAttrs(attrs))
}

func (l *logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return l
	}
	return l.with(logOp{group: name}, l.h.WithGroup(name))
}

func (l *logHandler) with(op logOp, h slog.Handler) *logHandler {
	ops := make([]logOp, len(l.ops), len(l.ops)+1)
	copy(ops, l.ops)
	return &logHandler{
		base: l.base,
		h:    h,
		ops:  append(ops, op),
		grp:  l.grp || op.group != "",
		key:  l.key,
	}
}
