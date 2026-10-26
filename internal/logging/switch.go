package logging

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// Switch is a slog.Handler whose destination can be replaced while it is in
// use. main creates the app logger on it before the data dir is known
// (stderr), hands that logger to Wails and to callbacks that may run on other
// goroutines at once (the second-instance notification), and later points it
// at the log file with Set. Nothing reassigns a logger variable, so there is
// no data race, and every logger derived with With/WithGroup follows the
// switch too.
type Switch struct {
	cur   *atomic.Pointer[target]
	ops   []func(slog.Handler) slog.Handler
	cache *atomic.Pointer[derived]
}

type target struct{ h slog.Handler }

// derived caches ops applied to one target, rebuilt after a Set.
type derived struct {
	base *target
	h    slog.Handler
}

// NewSwitch returns a Switch writing to h.
func NewSwitch(h slog.Handler) *Switch {
	s := &Switch{cur: &atomic.Pointer[target]{}, cache: &atomic.Pointer[derived]{}}
	s.cur.Store(&target{h: h})
	return s
}

// Set sends every following record (of this Switch and of the loggers
// derived from it) to h.
func (s *Switch) Set(h slog.Handler) { s.cur.Store(&target{h: h}) }

func (s *Switch) handler() slog.Handler {
	base := s.cur.Load()
	if len(s.ops) == 0 {
		return base.h
	}
	if d := s.cache.Load(); d != nil && d.base == base {
		return d.h
	}
	h := base.h
	for _, op := range s.ops {
		h = op(h)
	}
	s.cache.Store(&derived{base: base, h: h})
	return h
}

func (s *Switch) Enabled(ctx context.Context, l slog.Level) bool {
	return s.handler().Enabled(ctx, l)
}

func (s *Switch) Handle(ctx context.Context, r slog.Record) error {
	return s.handler().Handle(ctx, r)
}

func (s *Switch) with(op func(slog.Handler) slog.Handler) *Switch {
	ops := append(append([]func(slog.Handler) slog.Handler(nil), s.ops...), op)
	return &Switch{cur: s.cur, ops: ops, cache: &atomic.Pointer[derived]{}}
}

func (s *Switch) WithAttrs(attrs []slog.Attr) slog.Handler {
	return s.with(func(h slog.Handler) slog.Handler { return h.WithAttrs(attrs) })
}

func (s *Switch) WithGroup(name string) slog.Handler {
	return s.with(func(h slog.Handler) slog.Handler { return h.WithGroup(name) })
}
