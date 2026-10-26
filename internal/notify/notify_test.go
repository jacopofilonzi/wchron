package notify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/state"
)

type toast struct{ title, body string }

type fakeToaster struct {
	mu     sync.Mutex
	toasts []toast
}

func (f *fakeToaster) Toast(title, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toasts = append(f.toasts, toast{title, body})
	return nil
}

type sent struct {
	channel, target, text string
}

type fakeSender struct {
	channels map[string]bool // id → exists
	fail     map[string]error
	sends    []sent
}

func (f *fakeSender) SendInfo(id string) (provider.SendInfo, bool) {
	return provider.SendInfo{Name: id, Connected: true}, f.channels[id]
}

func (f *fakeSender) Send(_ context.Context, id string, target json.RawMessage, msg provider.Message) error {
	f.sends = append(f.sends, sent{id, string(target), msg.Text})
	return f.fail[id]
}

type memStore struct {
	cfg config.Config
	st  map[string]state.ScheduleState
}

func (m *memStore) Config() config.Config                       { return m.cfg }
func (m *memStore) ScheduleState(id string) state.ScheduleState { return m.st[id] }
func (m *memStore) UpdateScheduleState(id string, change func(*state.ScheduleState)) {
	s := m.st[id]
	change(&s)
	m.st[id] = s
}

type env struct {
	n      *Notifier
	toast  *fakeToaster
	sender *fakeSender
	store  *memStore
	clk    *clock.Fake
}

func newEnv(errRecipients ...config.Recipient) *env {
	cfg := config.Default()
	cfg.Settings.Language = i18n.EN
	cfg.Settings.ErrorRecipients = errRecipients
	// A schedule with its own recipient: it must never get an error text.
	cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales", Recipients: []config.Recipient{
		{ID: "r", ChannelID: "wa", Target: json.RawMessage(`{"to":"customer"}`), Verified: config.VerifiedYes}}}}
	e := &env{toast: &fakeToaster{}, sender: &fakeSender{channels: map[string]bool{"wa": true, "wa2": true}, fail: map[string]error{}},
		store: &memStore{cfg: cfg, st: map[string]state.ScheduleState{}}, clk: clock.NewFake(time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC))}
	e.n = New(Options{Toaster: e.toast, Sender: e.sender, Store: e.store, Clock: e.clk, Location: time.UTC})
	return e
}

func me(id, ch string) config.Recipient {
	return config.Recipient{ID: id, ChannelID: ch, Target: json.RawMessage(`{"to":"me-` + id + `"}`), DisplayName: "Me " + id, Verified: config.VerifiedYes}
}

func failed(ps ...*apperr.Error) Report {
	return Report{ScheduleID: "s", ScheduleName: "Sales", Slot: time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC), Status: state.ResultFailed, Problems: ps}
}

func notFound(path string) *apperr.Error {
	return apperr.New(apperr.FileNotFound, nil).With("schedule", "Sales").With("path", path)
}

func (e *env) report(r Report) []*apperr.Error { return e.n.Report(context.Background(), r) }

func TestDedupAndResolved(t *testing.T) {
	e := newEnv(me("e1", "wa"))
	ok := Report{ScheduleID: "s", ScheduleName: "Sales", Slot: e.clk.Now(), Status: state.ResultOK}

	// Success with nothing reported before: silent.
	e.report(ok)
	if len(e.toast.toasts)+len(e.sender.sends) != 0 {
		t.Fatal("a success must be silent")
	}
	// First failure: toast + error recipient.
	e.report(failed(notFound(`C:\a.xlsx`)))
	if len(e.toast.toasts) != 1 || len(e.sender.sends) != 1 {
		t.Fatalf("first failure: %d toasts, %d sends", len(e.toast.toasts), len(e.sender.sends))
	}
	first := e.toast.toasts[0]
	if first.title != "wchron: Sales not sent" || !strings.Contains(first.body, "Send of 2026-10-26 14:00:") ||
		!strings.Contains(first.body, `no file matches C:\a.xlsx`) {
		t.Fatalf("toast %+v", first)
	}
	if !strings.HasPrefix(e.sender.sends[0].text, "wchron: Sales not sent\nSend of") || e.sender.sends[0].target != `{"to":"me-e1"}` {
		t.Fatalf("message %+v", e.sender.sends[0])
	}
	// Same problem within 6 h (other params may change): suppressed.
	e.clk.Advance(5 * time.Hour)
	e.report(failed(notFound(`C:\a.xlsx`).With("count", "2")))
	if len(e.toast.toasts) != 1 {
		t.Fatal("same problem within 6 h must not be repeated")
	}
	// After 6 h: repeated, with the repeat note.
	e.clk.Advance(time.Hour)
	e.report(failed(notFound(`C:\a.xlsx`)))
	if len(e.toast.toasts) != 2 || !strings.Contains(e.toast.toasts[1].body, "Same problems as in the previous report") {
		t.Fatalf("repeat: %+v", e.toast.toasts)
	}
	// A different problem: at once.
	e.clk.Advance(time.Minute)
	e.report(failed(notFound(`C:\b.xlsx`)))
	if len(e.toast.toasts) != 3 || strings.Contains(e.toast.toasts[2].body, "Same problems") {
		t.Fatalf("changed: %+v", e.toast.toasts)
	}
	// Skip reasons don't count, and don't resolve.
	e.report(Report{ScheduleID: "s", ScheduleName: "Sales", Slot: e.clk.Now(), Status: state.ResultSkipped,
		Problems: []*apperr.Error{apperr.New(apperr.SlotLate, nil)}})
	if len(e.toast.toasts) != 3 {
		t.Fatal("slot.late must not notify")
	}
	// Success again: resolved notice, once.
	e.report(ok)
	e.report(ok)
	if len(e.toast.toasts) != 4 || e.toast.toasts[3].title != "wchron: Sales works again" || len(e.sender.sends) != 4 ||
		!strings.Contains(e.sender.sends[3].text, "went through again") {
		t.Fatalf("resolved: %+v / %+v", e.toast.toasts, e.sender.sends)
	}
	if st := e.store.st["s"]; st.NotifiedSignature != "" || !st.NotifiedAt.IsZero() {
		t.Fatalf("memory not cleared: %+v", st)
	}
	// Schedule recipients never get anything from notify.
	for _, s := range e.sender.sends {
		if strings.Contains(s.target, "customer") {
			t.Fatal("a schedule recipient got an error text")
		}
	}
}

func TestPartialAndWarningTitlesAndLongLists(t *testing.T) {
	e := newEnv()
	var ps []*apperr.Error
	for _, p := range []string{`C:\1`, `C:\2`, `C:\3`, `C:\4`, `C:\5`, `C:\6`} {
		ps = append(ps, notFound(p))
	}
	r := failed(ps...)
	r.Status = state.ResultPartial
	e.report(r)
	got := e.toast.toasts[0]
	if got.title != "wchron: Sales sent only in part" || strings.Count(got.body, "•") != ToastLines ||
		!strings.Contains(got.body, "and 2 more") {
		t.Fatalf("toast %+v", got)
	}
	r = Report{ScheduleID: "w", ScheduleName: "W", Slot: e.clk.Now(), Status: state.ResultOK,
		Problems: []*apperr.Error{apperr.New(apperr.SendTooLarge, nil)}}
	e.report(r)
	if e.toast.toasts[1].title != "wchron: W sent with warnings" {
		t.Fatalf("warning title %q", e.toast.toasts[1].title)
	}
}

func TestErrorRecipientFailures(t *testing.T) {
	bad := me("e2", "wa2")
	unverified := me("e3", "wa")
	unverified.Verified = config.VerifiedNo
	gone := me("e4", "deleted")
	e := newEnv(me("e1", "wa"), bad, unverified, gone)
	e.sender.fail["wa2"] = apperr.New(apperr.SendNotConnected, errors.New("down"))

	fails := e.report(failed(notFound(`C:\a`)))
	if len(fails) != 2 || fails[0].Code != apperr.NotifyFailed || fails[0].Params["recipient"] != "Me e2" ||
		fails[1].Params["recipient"] != "Me e4" {
		t.Fatalf("failures %+v", fails)
	}
	// e1 and e2 attempted, e3 (verified no) and e4 (no channel) not sent.
	if len(e.sender.sends) != 2 {
		t.Fatalf("sends %+v", e.sender.sends)
	}
	// The run toast, then the toast about the undelivered reports.
	if len(e.toast.toasts) != 2 || e.toast.toasts[1].title != "wchron: error report not delivered" ||
		!strings.Contains(e.toast.toasts[1].body, "Me e2") {
		t.Fatalf("toasts %+v", e.toast.toasts)
	}
}

func TestManualIsToastOnly(t *testing.T) {
	e := newEnv(me("e1", "wa"))
	r := failed(notFound(`C:\a`))
	r.Manual = true
	e.report(r)
	e.report(r) // every time, no dedup
	if len(e.toast.toasts) != 2 || len(e.sender.sends) != 0 || !strings.Contains(e.toast.toasts[0].body, "Send now (") {
		t.Fatalf("toasts %+v, sends %+v", e.toast.toasts, e.sender.sends)
	}
	if e.store.st["s"].NotifiedSignature != "" {
		t.Fatal("a manual run must not touch the memory")
	}
	r.Status, r.Problems = state.ResultOK, nil
	e.report(r)
	if len(e.toast.toasts) != 2 {
		t.Fatal("a manual success is silent")
	}
}

func TestSignature(t *testing.T) {
	a := apperr.New(apperr.SendFailed, nil).With("file", "a").With("recipient", "x")
	b := apperr.New(apperr.FileNotFound, nil).With("path", "p")
	if Signature([]*apperr.Error{a, b}) != Signature([]*apperr.Error{b, a, a}) {
		t.Fatal("order and duplicates must not matter")
	}
	c := apperr.New(apperr.SendFailed, nil).With("file", "a").With("recipient", "y")
	if Signature([]*apperr.Error{a}) == Signature([]*apperr.Error{c}) {
		t.Fatal("another recipient is another problem")
	}
}
