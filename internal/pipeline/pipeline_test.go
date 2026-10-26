package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/notify"
	"github.com/jacopofilonzi/wchron/internal/processor"
	"github.com/jacopofilonzi/wchron/internal/processor/passthrough"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/provider/fake"
	"github.com/jacopofilonzi/wchron/internal/scheduler"
	"github.com/jacopofilonzi/wchron/internal/state"
)

// memStore is the app store: the configuration, deep-copied.
type memStore struct {
	mu  sync.Mutex
	cfg config.Config
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
	s.cfg = next
	return nil
}

type reporter struct {
	mu      sync.Mutex
	reports []notify.Report
}

func (r *reporter) Report(_ context.Context, rep notify.Report) []*apperr.Error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, rep)
	return nil
}

var slot = time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC)

type env struct {
	t        *testing.T
	p        *Pipeline
	fp       *fake.Provider
	m        *provider.Manager
	store    *memStore
	clk      *clock.Fake
	rep      *reporter
	tmp, src string

	mu     sync.Mutex
	sleeps []time.Duration
	// verifyOK lets the pre-send check answer (at connection it is postponed).
	verifyOK atomic.Bool
}

func newEnv(t *testing.T, cfg config.Config) *env {
	t.Helper()
	cfg.Settings.Language = i18n.EN
	cfg.Normalize()
	e := &env{t: t, fp: fake.New(), store: &memStore{cfg: cfg}, clk: clock.NewFake(slot.Add(30 * time.Second)),
		rep: &reporter{}, tmp: t.TempDir(), src: t.TempDir()}
	e.fp.OnOpen = func(c *fake.Channel) {
		c.Validate = func(raw json.RawMessage) (json.RawMessage, string, error) {
			if !e.verifyOK.Load() {
				return raw, "", apperr.New(apperr.RecipientCheckUnavailable, errors.New("later"))
			}
			if strings.Contains(string(raw), "bad") {
				return nil, "", apperr.New(apperr.SendRecipientInvalid, provider.ErrTargetInvalid)
			}
			return raw, "Checked", nil
		}
	}
	e.m = provider.NewManager(provider.Options{
		Registry:  provider.NewRegistry(e.fp),
		Dir:       t.TempDir(),
		Store:     e.store,
		Clock:     e.clk,
		AfterFunc: func(time.Duration, func()) func() bool { return func() bool { return true } },
		SendPause: func() time.Duration { return 3 * time.Second },
		Sleep: func(ctx context.Context, d time.Duration) error {
			e.mu.Lock()
			e.sleeps = append(e.sleeps, d)
			e.mu.Unlock()
			return ctx.Err()
		},
	})
	t.Cleanup(e.m.Close)
	e.m.Start()
	for _, c := range cfg.Channels {
		id := c.ID
		deadline := time.Now().Add(3 * time.Second)
		for info, _ := e.m.SendInfo(id); !info.Connected; info, _ = e.m.SendInfo(id) {
			if time.Now().After(deadline) {
				t.Fatalf("channel %s not connected", id)
			}
			time.Sleep(time.Millisecond)
		}
	}
	time.Sleep(20 * time.Millisecond) // the postponed checks at connection
	e.verifyOK.Store(true)
	n := 0
	e.p = New(Options{Store: e.store, Channels: e.m, Processors: processor.NewRegistry(passthrough.New()), Reporter: e.rep,
		Clock: e.clk, Location: time.UTC, TempDir: e.tmp, NewRunID: func() string { n++; return fmt.Sprintf("run-%d", n) }})
	return e
}

func (e *env) file(name, content string) string {
	p := filepath.Join(e.src, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) run(id string) scheduler.Result {
	return e.p.Run(context.Background(), scheduler.Job{ScheduleID: id, ScheduleName: id, Slot: slot, Deadline: slot.Add(scheduler.Tolerance)})
}

// sends is "target file caption" for every send on a channel, in order.
func (e *env) sends(ch string) []string {
	var out []string
	c := e.fp.Channel(ch)
	if c == nil {
		return nil
	}
	for _, s := range c.SentMessages() {
		var tg struct{ To string }
		_ = json.Unmarshal(s.Target, &tg)
		out = append(out, fmt.Sprintf("%s %s %q", tg.To, s.Msg.Attachments[0].Name, s.Msg.Text))
	}
	return out
}

func (e *env) assertTempEmpty() {
	e.t.Helper()
	es, err := os.ReadDir(e.tmp)
	if err != nil || len(es) != 0 {
		e.t.Fatalf("temp dir not clean: %v %v", es, err)
	}
}

func codes(ps []*apperr.Error) string {
	var out []string
	for _, p := range ps {
		out = append(out, string(p.Code))
	}
	return strings.Join(out, ",")
}

func chn(id string) config.Channel {
	return config.Channel{ID: id, Provider: fake.Type, Name: "Ch " + id, Enabled: true}
}

func rcp(id, ch string, v config.Verified) config.Recipient {
	return config.Recipient{ID: id, ChannelID: ch, Target: json.RawMessage(`{"to":"` + id + `"}`), DisplayName: "R " + id, Verified: v}
}

func spec(id, path string) config.FileSpec {
	return config.FileSpec{ID: id, PathPattern: path, Match: config.MatchError, Processor: config.ProcessorPassthrough}
}

func strp(s string) *string { return &s }

func TestOrderCaptionsAndPauses(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{chn("a"), chn("b")}})
	f1, f2, f3 := e.file("one.pdf", "1"), e.file("two.pdf", "2"), e.file("three.pdf", "3")
	own := spec("f3", f3)
	own.Caption = strp("own {date:DD/MM}")
	e.store.cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales", Enabled: true, Caption: "Report {date:DD/MM HH:mm}",
		Files:      []config.FileSpec{spec("f1", f1), spec("f2", f2), own},
		Recipients: []config.Recipient{rcp("A", "a", config.VerifiedYes), rcp("C", "b", config.VerifiedYes), rcp("B", "a", config.VerifiedYes)}}}

	res := e.run("s")
	if res.Status != state.ResultOK || len(res.Problems) != 0 {
		t.Fatalf("result %+v", res)
	}
	want := []string{`A one.pdf "Report 26/10 14:00"`, `A two.pdf ""`, `A three.pdf "own 26/10"`,
		`B one.pdf "Report 26/10 14:00"`, `B two.pdf ""`, `B three.pdf "own 26/10"`}
	if got := e.sends("a"); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("channel a:\n%v\nwant\n%v", got, want)
	}
	if got := e.sends("b"); len(got) != 3 || got[0] != `C one.pdf "Report 26/10 14:00"` {
		t.Fatalf("channel b: %v", got)
	}
	// A pause before every send on a channel but its first: 5 on a, 2 on b.
	if len(e.sleeps) != 7 {
		t.Fatalf("sleeps %v", e.sleeps)
	}
	if len(e.rep.reports) != 1 || e.rep.reports[0].Status != state.ResultOK {
		t.Fatalf("reports %+v", e.rep.reports)
	}
	e.assertTempEmpty()
	// The originals are untouched.
	if b, _ := os.ReadFile(f1); string(b) != "1" {
		t.Fatal("original changed")
	}
}

func TestParallelChannelsSerializedAcrossRuns(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{chn("a"), chn("b")}})
	f := e.file("x.pdf", "x")
	for _, id := range []string{"s1", "s2"} {
		e.store.cfg.Schedules = append(e.store.cfg.Schedules, config.Schedule{ID: id, Name: id, Enabled: true,
			Files:      []config.FileSpec{spec("f", f), spec("g", f)},
			Recipients: []config.Recipient{rcp(id+"a", "a", config.VerifiedYes), rcp(id+"b", "b", config.VerifiedYes)}})
	}
	var mu sync.Mutex
	active, peak := map[string]int{}, map[string]int{}
	both := 0
	for _, ch := range []string{"a", "b"} {
		e.fp.Channel(ch).Edit(func(c *fake.Channel) {
			c.OnSend = func(json.RawMessage, provider.Message) error {
				mu.Lock()
				active[ch]++
				peak[ch] = max(peak[ch], active[ch])
				if active["a"] > 0 && active["b"] > 0 {
					both++
				}
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
				mu.Lock()
				active[ch]--
				mu.Unlock()
				return nil
			}
		})
	}
	var wg sync.WaitGroup
	for _, id := range []string{"s1", "s2"} {
		wg.Add(1)
		go func() { defer wg.Done(); e.run(id) }()
	}
	wg.Wait()
	if peak["a"] != 1 || peak["b"] != 1 {
		t.Fatalf("concurrent sends on one channel: %v", peak)
	}
	if both == 0 {
		t.Fatal("the two channels never sent in parallel")
	}
	if len(e.sends("a")) != 4 || len(e.sends("b")) != 4 {
		t.Fatalf("sends a %v b %v", e.sends("a"), e.sends("b"))
	}
	e.assertTempEmpty()
}

func TestCapabilitySkipsAndPartial(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{chn("a")}})
	small, big := e.file("small.pdf", "s"), e.file("big.pdf", strings.Repeat("b", 2048))
	e.fp.Caps.MaxFileSize = 1024
	e.fp.Caps.MaxCaptionLen = 10
	longCap := spec("l", small)
	longCap.Caption = strp("a caption far too long")
	e.store.cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales", Enabled: true, Caption: "short",
		Files:      []config.FileSpec{spec("b", big), spec("s", small), longCap},
		Recipients: []config.Recipient{rcp("A", "a", config.VerifiedYes)}}}
	res := e.run("s")
	if res.Status != state.ResultPartial || codes(res.Problems) != "send.too_large,send.caption_too_long" {
		t.Fatalf("result %s %s", res.Status, codes(res.Problems))
	}
	p := res.Problems[0].Params
	if p["file"] != "big.pdf" || p["size"] != "2 KB" || p["max"] != "1 KB" || p["recipient"] != "R A" || p["channel"] != "Ch a" || p["schedule"] != "Sales" {
		t.Fatalf("params %v", p)
	}
	// The big file was skipped, so the schedule caption goes on the first file sent.
	if got := e.sends("a"); fmt.Sprint(got) != `[A small.pdf "short"]` {
		t.Fatalf("sends %v", got)
	}
	// A text-only channel.
	e.fp.Caps.Files = false
	res = e.run("s")
	if res.Status != state.ResultFailed || codes(res.Problems) != "send.files_unsupported" {
		t.Fatalf("files unsupported: %s %s", res.Status, codes(res.Problems))
	}
}

func TestRecipientChecks(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{chn("a"), chn("b")}})
	f := e.file("x.pdf", "x")
	bad := rcp("bad", "a", config.VerifiedUnknown)
	e.store.cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales", Enabled: true,
		Files: []config.FileSpec{spec("f", f)},
		Recipients: []config.Recipient{rcp("new", "a", config.VerifiedUnknown), bad, rcp("no", "a", config.VerifiedNo),
			rcp("off", "b", config.VerifiedYes), rcp("gone", "zz", config.VerifiedYes)}}}
	e.fp.Channel("b").SetStatus(provider.Status{State: provider.StateDisconnected})
	res := e.run("s")
	if res.Status != state.ResultPartial {
		t.Fatalf("status %s", res.Status)
	}
	got := map[string]string{}
	for _, p := range res.Problems {
		got[p.Params["recipient"]] = string(p.Code)
	}
	want := map[string]string{"R bad": "send.recipient_invalid", "R no": "send.recipient_invalid", "R off": "send.not_connected",
		"R gone": "recipient.channel_missing"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("problems %v", got)
	}
	if s := e.sends("a"); fmt.Sprint(s) != `[new x.pdf ""]` {
		t.Fatalf("sends %v", s)
	}
	// The checks before the send were stored.
	rs := e.store.Config().Schedules[0].Recipients
	if rs[0].Verified != config.VerifiedYes || rs[0].DisplayName != "Checked" || rs[1].Verified != config.VerifiedNo {
		t.Fatalf("stored %+v", rs[:2])
	}
}

func TestFailuresDontStopOthers(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{chn("a")}})
	f1, f2 := e.file("one.pdf", "1"), e.file("two.pdf", "2")
	e.store.cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales", Enabled: true,
		Files:      []config.FileSpec{spec("m", filepath.Join(e.src, "missing-*.pdf")), spec("1", f1), spec("2", f2)},
		Recipients: []config.Recipient{rcp("A", "a", config.VerifiedYes), rcp("B", "a", config.VerifiedYes)}}}
	e.fp.Channel("a").Edit(func(c *fake.Channel) {
		c.OnSend = func(raw json.RawMessage, msg provider.Message) error {
			if strings.Contains(string(raw), "A") && msg.Attachments[0].Name == "one.pdf" {
				return apperr.New(apperr.SendFailed, errors.New("no ack"))
			}
			return nil
		}
	})
	res := e.run("s")
	if res.Status != state.ResultPartial || codes(res.Problems) != "file.not_found,send.failed" ||
		res.Err.Code != apperr.FileNotFound || res.Problems[1].Params["file"] != "one.pdf" {
		t.Fatalf("result %s %s %+v", res.Status, codes(res.Problems), res.Err)
	}
	if len(e.sends("a")) != 4 {
		t.Fatalf("sends %v", e.sends("a"))
	}
	e.assertTempEmpty()
	// Nothing to send at all: failed, temp still clean.
	e.store.cfg.Schedules[0].Files = e.store.cfg.Schedules[0].Files[:1]
	if res := e.run("s"); res.Status != state.ResultFailed {
		t.Fatalf("status %s", res.Status)
	}
	e.assertTempEmpty()
}

func TestLateBeforeTheFirstSend(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{chn("a")}})
	f := e.file("x.pdf", "x")
	e.store.cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales", Enabled: true,
		Files: []config.FileSpec{spec("f", f)}, Recipients: []config.Recipient{rcp("A", "a", config.VerifiedYes)}}}
	e.clk.Set(slot.Add(scheduler.Tolerance + time.Second))
	res := e.run("s")
	if res.Status != state.ResultSkipped || res.Err.Code != apperr.SlotLate || res.Err.Params["slot"] != "2026-10-26 14:00" {
		t.Fatalf("result %+v", res)
	}
	if len(e.sends("a")) != 0 || len(e.rep.reports) != 0 {
		t.Fatal("a late run sent or reported")
	}
	e.assertTempEmpty()
	// A manual run has no deadline.
	res = e.p.Run(context.Background(), scheduler.Job{ScheduleID: "s", Slot: slot, Manual: true})
	if res.Status != state.ResultOK || len(e.sends("a")) != 1 || !e.rep.reports[0].Manual {
		t.Fatalf("manual %+v", res)
	}
}

func TestCancellationStopsBetweenSends(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{chn("a")}})
	f1, f2 := e.file("one.pdf", "1"), e.file("two.pdf", "2")
	e.store.cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales", Enabled: true,
		Files: []config.FileSpec{spec("1", f1), spec("2", f2)}, Recipients: []config.Recipient{rcp("A", "a", config.VerifiedYes)}}}
	ctx, cancel := context.WithCancel(context.Background())
	e.fp.Channel("a").Edit(func(c *fake.Channel) {
		c.OnSend = func(json.RawMessage, provider.Message) error { cancel(); return nil }
	})
	res := e.p.Run(ctx, scheduler.Job{ScheduleID: "s", Slot: slot, Deadline: slot.Add(scheduler.Tolerance)})
	if res.Status != state.ResultPartial || len(e.sends("a")) != 1 || len(e.rep.reports) != 0 {
		t.Fatalf("result %+v, sends %v, reports %d", res, e.sends("a"), len(e.rep.reports))
	}
	e.assertTempEmpty()
}

func TestScheduleGoneAndTempUnavailable(t *testing.T) {
	e := newEnv(t, config.Config{Channels: []config.Channel{chn("a")}})
	if res := e.run("nope"); res.Status != state.ResultSkipped || res.Err.Code != apperr.ScheduleNotFound {
		t.Fatalf("gone %+v", res)
	}
	e.store.cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales", Enabled: true,
		Files: []config.FileSpec{spec("f", e.file("x.pdf", "x"))}, Recipients: []config.Recipient{rcp("A", "a", config.VerifiedYes)}}}
	// The temp root is a file: nothing can be created under it.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e.p.o.TempDir = blocker
	if res := e.run("s"); res.Status != state.ResultFailed || res.Err.Code != apperr.TempUnavailable || len(e.sends("a")) != 0 {
		t.Fatalf("temp %+v", res)
	}
}

func TestCleanTempAndMask(t *testing.T) {
	e := newEnv(t, config.Config{})
	if err := os.MkdirAll(filepath.Join(e.tmp, "old-run", "0-0"), 0o700); err != nil {
		t.Fatal(err)
	}
	e.p.CleanTemp()
	e.assertTempEmpty()
	if got := maskParams(map[string]string{"recipient": "+39 333 1234567", "file": "a.pdf"}); got["recipient"] != "***567" || got["file"] != "a.pdf" {
		t.Fatalf("mask %v", got)
	}
}
