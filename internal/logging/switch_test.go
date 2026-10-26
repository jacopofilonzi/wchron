package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func TestSwitchFollowsSetAlsoInDerivedLoggers(t *testing.T) {
	var first, second bytes.Buffer
	sw := NewSwitch(slog.NewTextHandler(&first, nil))
	log := slog.New(sw)
	child := log.With("channel", "a").WithGroup("g")
	log.Info("one")
	child.Info("two", "k", 1)
	sw.Set(slog.NewTextHandler(&second, &slog.HandlerOptions{Level: slog.LevelWarn}))
	log.Info("dropped")
	child.Warn("three", "k", 2)
	if s := first.String(); !strings.Contains(s, "msg=one") || !strings.Contains(s, "channel=a g.k=1") {
		t.Fatalf("first: %q", s)
	}
	if s := second.String(); strings.Contains(s, "dropped") || !strings.Contains(s, "channel=a g.k=2") {
		t.Fatalf("second: %q", s)
	}
}

// Run with -race where cgo is available: logging while Set runs.
func TestSwitchConcurrentSet(t *testing.T) {
	sw := NewSwitch(slog.NewTextHandler(&safeBuf{}, nil))
	log := slog.New(sw).With("x", 1)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for j := 0; j < 200; j++ {
				log.Info("m")
			}
		})
	}
	for j := 0; j < 50; j++ {
		sw.Set(slog.NewJSONHandler(&safeBuf{}, nil))
	}
	wg.Wait()
}

type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
