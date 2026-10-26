package provider_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/provider/fake"
)

// memStore is an in-memory ConfigStore.
type memStore struct {
	mu      sync.Mutex
	cfg     config.Config
	failing bool
	saves   int
}

func clone(c config.Config) config.Config {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	var out config.Config
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

func (s *memStore) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.cfg)
}

func (s *memStore) UpdateConfig(change func(*config.Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.cfg)
	if err := change(&next); err != nil {
		return err
	}
	if s.failing {
		return apperr.New(apperr.ConfigWriteFailed, errors.New("disk full"))
	}
	s.cfg = next
	s.saves++
	return nil
}

// timers is a manual AfterFunc: the test fires the pending reconnects.
type timers struct {
	mu      sync.Mutex
	pending []timer
	delays  []time.Duration
}

type timer struct {
	f       func()
	stopped *bool
}

func (t *timers) after(d time.Duration, f func()) func() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	stopped := new(bool)
	t.pending = append(t.pending, timer{f: f, stopped: stopped})
	t.delays = append(t.delays, d)
	return func() bool {
		t.mu.Lock()
		defer t.mu.Unlock()
		was := !*stopped
		*stopped = true
		return was
	}
}

// fire runs the pending, not stopped timers.
func (t *timers) fire() int {
	t.mu.Lock()
	p := t.pending
	t.pending = nil
	t.mu.Unlock()
	n := 0
	for _, x := range p {
		t.mu.Lock()
		stopped := *x.stopped
		t.mu.Unlock()
		if !stopped {
			n++
			x.f()
		}
	}
	return n
}

func (t *timers) Delays() []time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]time.Duration(nil), t.delays...)
}

type env struct {
	m      *provider.Manager
	p      *fake.Provider
	store  *memStore
	timers *timers
	dir    string

	mu       sync.Mutex
	lists    [][]provider.ChannelInfo
	pairings []provider.PairingEvent
	reloads  int
	outdated []string
}

func newEnv(t *testing.T, cfg config.Config) *env {
	t.Helper()
	cfg.Normalize()
	e := &env{p: fake.New(), store: &memStore{cfg: cfg}, timers: &timers{}, dir: t.TempDir()}
	n := 0
	e.m = provider.NewManager(provider.Options{
		Registry: provider.NewRegistry(e.p),
		Dir:      e.dir,
		Store:    e.store,
		OnChange: func(l []provider.ChannelInfo) {
			e.mu.Lock()
			e.lists = append(e.lists, l)
			e.mu.Unlock()
		},
		OnPairing: func(p provider.PairingEvent) {
			e.mu.Lock()
			e.pairings = append(e.pairings, p)
			e.mu.Unlock()
		},
		OnSchedulesChanged: func() { e.mu.Lock(); e.reloads++; e.mu.Unlock() },
		OnOutdated:         func(id string) { e.mu.Lock(); e.outdated = append(e.outdated, id); e.mu.Unlock() },
		AfterFunc:          e.timers.after,
		NewID:              func() string { n++; return "new-" + string(rune('0'+n)) },
	})
	t.Cleanup(e.m.Close)
	return e
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (e *env) state(id string) provider.State {
	for _, c := range e.m.List() {
		if c.ID == id {
			return c.Status.State
		}
	}
	return ""
}

func ch(id string) config.Channel {
	return config.Channel{ID: id, Provider: fake.Type, Name: "Channel " + id, Enabled: true}
}

func rcp(id, channel, to string, v config.Verified) config.Recipient {
	return config.Recipient{ID: id, ChannelID: channel, Target: json.RawMessage(`{"to":"` + to + `"}`), Verified: v}
}

func TestStartOpensAndConnectsEnabledChannels(t *testing.T) {
	off := ch("b")
	off.Enabled = false
	unknown := ch("c")
	unknown.Provider = "carrier-pigeon"
	e := newEnv(t, config.Config{Channels: []config.Channel{ch("a"), off, unknown}})
	e.m.Start()
	waitFor(t, "a connected", func() bool { return e.state("a") == provider.StateConnected })
	if e.p.Channel("b") != nil {
		t.Fatal("a disabled channel must not be opened")
	}
	list := e.m.List()
	if len(list) != 3 || list[0].Status.Account != fake.Account || list[2].Status.State != provider.StateError ||
		list[2].Status.Error.Code != apperr.ChannelProviderUnknown || list[2].Status.Error.Params["channel"] != "Channel c" {
		t.Fatalf("list: %+v", list)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "a")); err != nil {
		t.Fatalf("channel dir: %v", err)
	}
}

func TestReconnectBackoffDoublesToMaxAndResets(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{ch("a")}})
	e.p.Unpaired = false
	e.m.Start()
	c := e.p.Channel("a")
	waitFor(t, "connected", func() bool { return e.state("a") == provider.StateConnected })
	c.Edit(func(c *fake.Channel) { c.ConnectErr = errors.New("offline") })
	// The connection drops: the manager retries with a growing delay.
	c.SetStatus(provider.Status{State: provider.StateDisconnected, Retry: true, Account: fake.Account})
	for i := 0; i < 8; i++ {
		waitFor(t, "a retry", func() bool { return len(e.timers.Delays()) > i })
		e.timers.fire()
	}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second,
		160 * time.Second, 300 * time.Second, 300 * time.Second}
	got := e.timers.Delays()[:8]
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("delays %v, want %v", got, want)
		}
	}
	// Back online: the next retry connects and the backoff starts over.
	waitFor(t, "a ninth retry", func() bool { return len(e.timers.Delays()) > 8 })
	c.Edit(func(c *fake.Channel) { c.ConnectErr = nil })
	e.timers.fire()
	waitFor(t, "connected", func() bool { return e.state("a") == provider.StateConnected })
	c.SetStatus(provider.Status{State: provider.StateDisconnected, Retry: true})
	waitFor(t, "a new retry", func() bool { return len(e.timers.Delays()) > 9 })
	if d := e.timers.Delays()[9]; d != 5*time.Second {
		t.Fatalf("after a success the backoff restarts at 5s, got %v", d)
	}
}

func TestNoReconnectForFinalStates(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{ch("a")}})
	e.m.Start()
	c := e.p.Channel("a")
	waitFor(t, "connected", func() bool { return e.state("a") == provider.StateConnected })
	for _, s := range []provider.Status{
		{State: provider.StateLoggedOut}, {State: provider.StateNotPaired},
		{State: provider.StateDisconnected, Retry: false}, {State: provider.StateOutdated},
	} {
		c.SetStatus(s)
	}
	if n := len(e.timers.Delays()); n != 0 {
		t.Fatalf("no reconnect expected, got %d", n)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.outdated) != 1 || e.outdated[0] != "a" {
		t.Fatalf("outdated hook: %v", e.outdated)
	}
}

func TestCreateRenameAndPairing(t *testing.T) {
	e := newEnv(t, config.Config{})
	e.p.Unpaired = true
	if _, err := e.m.CreateChannel(fake.Type, "  "); apperr.As(err).Code != apperr.ChannelNameRequired {
		t.Fatalf("empty name: %v", err)
	}
	if _, err := e.m.CreateChannel("nope", "x"); apperr.As(err).Code != apperr.ChannelProviderUnknown {
		t.Fatalf("unknown provider: %v", err)
	}
	info, err := e.m.CreateChannel(fake.Type, " Test ")
	if err != nil || info.ID != "new-1" || info.Name != "Test" || !info.Enabled {
		t.Fatalf("create: %+v %v", info, err)
	}
	if cfg := e.store.Config(); len(cfg.Channels) != 1 || cfg.Channels[0].Name != "Test" || string(cfg.Channels[0].Settings) != "{}" {
		t.Fatalf("saved: %+v", cfg.Channels)
	}
	waitFor(t, "not paired", func() bool { return e.state("new-1") == provider.StateNotPaired })

	if _, err := e.m.StartPairing("new-1"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "paired", func() bool { return e.state("new-1") == provider.StateConnected })
	waitFor(t, "pairing events", func() bool { e.mu.Lock(); defer e.mu.Unlock(); return len(e.pairings) == 2 })
	if e.pairings[0].Kind != provider.PairCode || e.pairings[0].Code != "fake-qr-1" || e.pairings[0].ChannelID != "new-1" ||
		e.pairings[1].Kind != provider.PairSuccess {
		t.Fatalf("pairing: %+v", e.pairings)
	}
	if e.m.List()[0].Pairing {
		t.Fatal("pairing flag must clear")
	}

	if _, err := e.m.RenameChannel("new-1", ""); apperr.As(err).Code != apperr.ChannelNameRequired {
		t.Fatalf("rename empty: %v", err)
	}
	if _, err := e.m.RenameChannel("zzz", "x"); apperr.As(err).Code != apperr.ChannelNotFound {
		t.Fatalf("rename missing: %v", err)
	}
	if info, err := e.m.RenameChannel("new-1", "Renamed"); err != nil || info.Name != "Renamed" || e.store.Config().Channels[0].Name != "Renamed" {
		t.Fatalf("rename: %+v %v", info, err)
	}
}

func TestPairingCancelAndTimeout(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{ch("a")}})
	e.p.Unpaired = true
	e.m.Start()
	waitFor(t, "not paired", func() bool { return e.state("a") == provider.StateNotPaired })
	c := e.p.Channel("a")
	c.Edit(func(c *fake.Channel) {
		c.PairEvents = []*provider.PairEvent{{Kind: provider.PairCode, Code: "q1"}, nil}
	})
	if _, err := e.m.StartPairing("a"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a code", func() bool { e.mu.Lock(); defer e.mu.Unlock(); return len(e.pairings) == 1 })
	if err := e.m.CancelPairing("a"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "cancelled", func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		return len(e.pairings) == 2 && e.pairings[1].Kind == provider.PairCancelled
	})

	c.Edit(func(c *fake.Channel) {
		c.PairEvents = []*provider.PairEvent{{Kind: provider.PairTimeout, Err: apperr.New(apperr.WAPairTimeout, nil)}}
	})
	if _, err := e.m.StartPairing("a"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "timeout", func() bool { e.mu.Lock(); defer e.mu.Unlock(); return len(e.pairings) == 3 })
	if p := e.pairings[2]; p.Kind != provider.PairTimeout || p.Error.Code != apperr.WAPairTimeout || p.Error.Params["channel"] != "Channel a" {
		t.Fatalf("timeout event: %+v", p)
	}
}

func TestLogout(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{ch("a")}})
	e.m.Start()
	waitFor(t, "connected", func() bool { return e.state("a") == provider.StateConnected })
	info, err := e.m.Logout("a")
	if err != nil || info.Status.State != provider.StateNotPaired || e.p.Channel("a").Logouts != 1 {
		t.Fatalf("logout: %+v %v", info, err)
	}
	if len(e.store.Config().Channels) != 1 {
		t.Fatal("logout keeps the channel")
	}
	if _, err := e.m.Logout("a"); apperr.As(err).Code != apperr.ChannelNotConnected {
		t.Fatalf("logout while not connected: %v", err)
	}
}

func TestUsageAndDeleteChannel(t *testing.T) {
	cfg := config.Config{
		Channels: []config.Channel{ch("a"), ch("b")},
		Settings: config.Settings{ErrorRecipients: []config.Recipient{rcp("e1", "a", "me", config.VerifiedYes), rcp("e2", "b", "me", config.VerifiedYes)}},
		Schedules: []config.Schedule{
			{ID: "s1", Name: "Only a", Enabled: true, Cadence: config.Cadence{Mode: config.CadenceFixed},
				Recipients: []config.Recipient{rcp("r1", "a", "x", config.VerifiedYes), rcp("r2", "a", "y", config.VerifiedYes)}},
			{ID: "s2", Name: "Mixed", Enabled: true, Cadence: config.Cadence{Mode: config.CadenceFixed},
				Recipients: []config.Recipient{rcp("r1", "a", "x", config.VerifiedYes), rcp("r2", "b", "y", config.VerifiedYes)}},
			{ID: "s3", Name: "Only b", Enabled: true, Cadence: config.Cadence{Mode: config.CadenceFixed},
				Recipients: []config.Recipient{rcp("r1", "b", "x", config.VerifiedYes)}},
		},
	}
	e := newEnv(t, cfg)
	e.m.Start()
	waitFor(t, "connected", func() bool { return e.state("a") == provider.StateConnected })
	if err := os.WriteFile(filepath.Join(e.dir, "a", "session.db"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	u, err := e.m.Usage("a")
	if err != nil || u.ErrorRecipients != 1 || len(u.Schedules) != 2 ||
		u.Schedules[0] != (provider.UsageSchedule{ID: "s1", Name: "Only a", Recipients: 2, Disabled: true}) ||
		u.Schedules[1] != (provider.UsageSchedule{ID: "s2", Name: "Mixed", Recipients: 1, Disabled: false}) {
		t.Fatalf("usage: %+v %v", u, err)
	}

	// A failed save keeps everything and the channel working.
	e.store.failing = true
	if _, err := e.m.DeleteChannel("a"); apperr.As(err).Code != apperr.ConfigWriteFailed {
		t.Fatalf("delete with a failing save: %v", err)
	}
	e.store.failing = false
	if len(e.store.Config().Channels) != 2 {
		t.Fatal("config must be unchanged")
	}
	waitFor(t, "reopened", func() bool { return e.state("a") == provider.StateConnected })
	// The first attempt already logged out (best effort comes first): pair again.
	if _, err := e.m.StartPairing("a"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "paired again", func() bool { return e.state("a") == provider.StateConnected })

	old := e.p.Channel("a")
	res, err := e.m.DeleteChannel("a")
	if err != nil || !res.WasPaired || !res.LoggedOut || !old.Closed {
		t.Fatalf("delete: %+v %v closed=%v", res, err, old.Closed)
	}
	got := e.store.Config()
	if len(got.Channels) != 1 || got.Channels[0].ID != "b" || len(got.Settings.ErrorRecipients) != 1 ||
		got.Settings.ErrorRecipients[0].ID != "e2" {
		t.Fatalf("channels/error recipients: %+v %+v", got.Channels, got.Settings.ErrorRecipients)
	}
	s1, s2, s3 := got.Schedules[0], got.Schedules[1], got.Schedules[2]
	if s1.Enabled || len(s1.Recipients) != 0 || !s2.Enabled || len(s2.Recipients) != 1 || s2.Recipients[0].ChannelID != "b" ||
		!s3.Enabled || len(s3.Recipients) != 1 {
		t.Fatalf("schedules: %+v", got.Schedules)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "a")); !os.IsNotExist(err) {
		t.Fatalf("channel dir must be gone: %v", err)
	}
	if len(e.m.List()) != 1 || e.reloads != 1 {
		t.Fatalf("list %+v reloads %d", e.m.List(), e.reloads)
	}
	if _, err := e.m.DeleteChannel("a"); apperr.As(err).Code != apperr.ChannelNotFound {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestDeleteDisconnectedChannelSkipsLogout(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{ch("a")}})
	e.m.Start()
	c := e.p.Channel("a")
	waitFor(t, "connected", func() bool { return e.state("a") == provider.StateConnected })
	c.SetStatus(provider.Status{State: provider.StateDisconnected, Account: fake.Account})
	res, err := e.m.DeleteChannel("a")
	if err != nil || !res.WasPaired || res.LoggedOut || c.Logouts != 0 {
		t.Fatalf("delete: %+v %v", res, err)
	}
}

func TestValidateTarget(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{ch("a")}})
	e.m.Start()
	fc := e.p.Channel("a")
	// Not connected yet: saved unverified, with a warning.
	fc.SetStatus(provider.Status{State: provider.StateDisconnected})
	chk, err := e.m.ValidateTarget(t.Context(), "a", json.RawMessage(`{"to":"x"}`))
	if err != nil || chk.Verified != config.VerifiedUnknown || chk.Warning == nil ||
		chk.Warning.Code != apperr.RecipientCheckUnavailable || chk.Warning.Params["channel"] != "Channel a" {
		t.Fatalf("unavailable: %+v %v", chk, err)
	}
	fc.SetStatus(provider.Status{State: provider.StateConnected})
	chk, err = e.m.ValidateTarget(t.Context(), "a", json.RawMessage(`{"to":"x"}`))
	if err != nil || chk.Verified != config.VerifiedYes || chk.DisplayName != "fake" || chk.Warning != nil {
		t.Fatalf("ok: %+v %v", chk, err)
	}
	fc.Edit(func(c *fake.Channel) {
		c.Validate = func(json.RawMessage) (json.RawMessage, string, error) {
			return nil, "", apperr.New(apperr.WANotOnWhatsApp, provider.ErrTargetInvalid).With("value", "x")
		}
	})
	if _, err := e.m.ValidateTarget(t.Context(), "a", json.RawMessage(`{}`)); apperr.As(err).Code != apperr.WANotOnWhatsApp {
		t.Fatalf("invalid: %v", err)
	}
	if _, err := e.m.ValidateTarget(t.Context(), "zz", nil); apperr.As(err).Code != apperr.ChannelNotFound {
		t.Fatalf("missing channel: %v", err)
	}
}

func TestPendingRecipientsAreVerifiedOnConnect(t *testing.T) {
	cfg := config.Config{
		Channels: []config.Channel{ch("a")},
		Settings: config.Settings{ErrorRecipients: []config.Recipient{rcp("e1", "a", "good", config.VerifiedUnknown)}},
		Schedules: []config.Schedule{{ID: "s1", Name: "S", Enabled: true, Cadence: config.Cadence{Mode: config.CadenceFixed},
			Recipients: []config.Recipient{
				rcp("r1", "a", "good", config.VerifiedUnknown),
				rcp("r2", "a", "bad", config.VerifiedUnknown),
				rcp("r3", "a", "flaky", config.VerifiedUnknown),
				rcp("r4", "a", "old", config.VerifiedYes),
			}}},
	}
	e := newEnv(t, cfg)
	var mu sync.Mutex
	calls := map[string]int{}
	validate := func(raw json.RawMessage) (json.RawMessage, string, error) {
		var t struct{ To string }
		_ = json.Unmarshal(raw, &t)
		mu.Lock()
		calls[t.To]++
		mu.Unlock()
		switch t.To {
		case "bad":
			return nil, "", apperr.New(apperr.WANotOnWhatsApp, provider.ErrTargetInvalid)
		case "flaky":
			return nil, "", apperr.New(apperr.ChannelNotConnected, errors.New("dropped"))
		}
		return json.RawMessage(`{"to":"` + t.To + `","n":1}`), "Name " + t.To, nil
	}
	e.p.OnOpen = func(c *fake.Channel) { c.Validate = validate }
	e.m.Start()
	waitFor(t, "checks", func() bool { mu.Lock(); defer mu.Unlock(); return calls["flaky"] == 1 })
	waitFor(t, "saved", func() bool { return e.store.Config().Schedules[0].Recipients[1].Verified == config.VerifiedNo })
	got := e.store.Config()
	rs := got.Schedules[0].Recipients
	if rs[0].Verified != config.VerifiedYes || rs[0].DisplayName != "Name good" || string(rs[0].Target) != `{"to":"good","n":1}` ||
		rs[1].Verified != config.VerifiedNo || rs[2].Verified != config.VerifiedUnknown || rs[3].Verified != config.VerifiedYes ||
		got.Settings.ErrorRecipients[0].Verified != config.VerifiedYes {
		t.Fatalf("recipients: %+v / %+v", rs, got.Settings.ErrorRecipients)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["good"] != 1 || calls["old"] != 0 {
		t.Fatalf("each target is checked once, verified ones never: %v", calls)
	}
}
