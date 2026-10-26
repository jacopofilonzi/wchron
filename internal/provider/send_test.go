package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/provider/fake"
)

// sendEnv is a Manager whose pauses are recorded instead of slept: Sleep
// advances the fake clock.
type sendEnv struct {
	m     *provider.Manager
	p     *fake.Provider
	store *memStore
	clk   *clock.Fake

	mu     sync.Mutex
	sleeps []time.Duration
}

func newSendEnv(t *testing.T, cfg config.Config, setup ...func(*fake.Provider)) *sendEnv {
	t.Helper()
	cfg.Normalize()
	e := &sendEnv{p: fake.New(), store: &memStore{cfg: cfg}, clk: clock.NewFake(time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC))}
	e.m = provider.NewManager(provider.Options{
		Registry:  provider.NewRegistry(e.p),
		Dir:       t.TempDir(),
		Store:     e.store,
		Clock:     e.clk,
		AfterFunc: (&timers{}).after,
		SendPause: func() time.Duration { return 3 * time.Second },
		Sleep: func(ctx context.Context, d time.Duration) error {
			e.mu.Lock()
			e.sleeps = append(e.sleeps, d)
			e.mu.Unlock()
			if d > 0 {
				e.clk.Advance(d)
			}
			return ctx.Err()
		},
	})
	for _, f := range setup {
		f(e.p)
	}
	t.Cleanup(e.m.Close)
	e.m.Start()
	for _, c := range cfg.Channels {
		id := c.ID
		waitFor(t, id+" connected", func() bool { info, _ := e.m.SendInfo(id); return info.Connected })
	}
	return e
}

func target(to string) json.RawMessage { return json.RawMessage(`{"to":"` + to + `"}`) }

func TestSendPausesBetweenSendsOnAChannel(t *testing.T) {
	e := newSendEnv(t, config.Config{Channels: []config.Channel{ch("a")}})
	ctx := context.Background()
	if err := e.m.Send(ctx, "a", target("1"), provider.Message{Text: "one"}); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(time.Second)
	if err := e.m.Send(ctx, "a", target("2"), provider.Message{Text: "two"}); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(time.Minute)
	if err := e.m.Send(ctx, "a", target("3"), provider.Message{Text: "three"}); err != nil {
		t.Fatal(err)
	}
	// First send: no wait; second: 3 s pause − 1 s already passed; third: long past.
	if fmt.Sprint(e.sleeps) != "[2s -57s]" {
		t.Fatalf("sleeps %v", e.sleeps)
	}
	if n := len(e.p.Channel("a").SentMessages()); n != 3 {
		t.Fatalf("sent %d", n)
	}
}

func TestSendSerializesPerChannelOnly(t *testing.T) {
	e := newSendEnv(t, config.Config{Channels: []config.Channel{ch("a"), ch("b")}})
	var mu sync.Mutex
	active := map[string]int{}
	maxActive := map[string]int{}
	release := make(chan struct{})
	both := make(chan struct{}, 4)
	for _, id := range []string{"a", "b"} {
		e.p.Channel(id).Edit(func(c *fake.Channel) {
			c.OnSend = func(json.RawMessage, provider.Message) error {
				mu.Lock()
				active[id]++
				maxActive[id] = max(maxActive[id], active[id])
				mu.Unlock()
				both <- struct{}{}
				<-release
				mu.Lock()
				active[id]--
				mu.Unlock()
				return nil
			}
		})
	}
	var wg sync.WaitGroup
	for i := range 4 {
		id := []string{"a", "b"}[i%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = e.m.Send(context.Background(), id, target("x"), provider.Message{})
		}()
	}
	// One send per channel starts at once: a and b in parallel.
	<-both
	<-both
	close(release)
	wg.Wait()
	if maxActive["a"] != 1 || maxActive["b"] != 1 {
		t.Fatalf("concurrent sends on one channel: %v", maxActive)
	}
}

func TestSendErrors(t *testing.T) {
	e := newSendEnv(t, config.Config{Channels: []config.Channel{ch("a")}})
	if err := e.m.Send(context.Background(), "zz", target("1"), provider.Message{}); apperr.As(err).Code != apperr.ChannelNotFound {
		t.Fatalf("unknown channel: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = e.m.Send(context.Background(), "a", target("1"), provider.Message{})
	if err := e.m.Send(ctx, "a", target("1"), provider.Message{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if _, ok := e.m.SendInfo("zz"); ok {
		t.Fatal("SendInfo of an unknown channel")
	}
	info, ok := e.m.SendInfo("a")
	if !ok || info.Name != "Channel a" || !info.Capabilities.Files || !info.Connected {
		t.Fatalf("info %+v", info)
	}
}

func TestVerifyRecipientStoresTheResult(t *testing.T) {
	cfg := config.Config{
		Channels: []config.Channel{ch("a")},
		Settings: config.Settings{ErrorRecipients: []config.Recipient{rcp("e1", "a", "good", config.VerifiedUnknown)}},
		Schedules: []config.Schedule{{ID: "s", Recipients: []config.Recipient{
			rcp("r1", "a", "good", config.VerifiedUnknown), rcp("r2", "a", "bad", config.VerifiedUnknown)}}},
	}
	// The check at connection is postponed (temporary failure), so the
	// recipients are still unverified when the send checks them.
	var connected atomic.Bool
	e := newSendEnv(t, cfg, func(p *fake.Provider) {
		p.OnOpen = func(c *fake.Channel) {
			c.Validate = func(raw json.RawMessage) (json.RawMessage, string, error) {
				if !connected.Load() {
					return raw, "", apperr.New(apperr.RecipientCheckUnavailable, errors.New("later"))
				}
				if string(raw) == string(target("bad")) {
					return nil, "", apperr.New(apperr.SendRecipientInvalid, provider.ErrTargetInvalid)
				}
				return raw, "Good one", nil
			}
		}
	})
	time.Sleep(50 * time.Millisecond) // let the postponed check at connection finish
	connected.Store(true)
	v, _, err := e.m.VerifyRecipient(context.Background(), "a", target("good"))
	if err != nil || v != config.VerifiedYes {
		t.Fatalf("good: %v %v", v, err)
	}
	v, _, err = e.m.VerifyRecipient(context.Background(), "a", target("bad"))
	if err != nil || v != config.VerifiedNo {
		t.Fatalf("bad: %v %v", v, err)
	}
	got := e.store.Config()
	rs := got.Schedules[0].Recipients
	if rs[0].Verified != config.VerifiedYes || rs[0].DisplayName != "Good one" || rs[1].Verified != config.VerifiedNo ||
		got.Settings.ErrorRecipients[0].Verified != config.VerifiedYes {
		t.Fatalf("stored %+v / %+v", rs, got.Settings.ErrorRecipients)
	}
	// Disconnected: unknown, nothing stored.
	e.p.Channel("a").SetStatus(provider.Status{State: provider.StateDisconnected})
	if v, _, err = e.m.VerifyRecipient(context.Background(), "a", target("other")); err == nil || v != config.VerifiedUnknown {
		t.Fatalf("offline: %v %v", v, err)
	}
}
