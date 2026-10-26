package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/state"
)

// memStore is the app store: routine flag and per-schedule state, with the
// history of every state written, to check what was recorded and how often.
type memStore struct {
	mu      sync.Mutex
	active  bool
	st      map[string]state.ScheduleState
	history map[string][]state.ScheduleState
}

func newMemStore() *memStore {
	return &memStore{active: true, st: map[string]state.ScheduleState{}, history: map[string][]state.ScheduleState{}}
}

func (m *memStore) RoutineActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

func (m *memStore) setActive(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = on
}

func (m *memStore) ScheduleState(id string) state.ScheduleState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st[id]
}

func (m *memStore) UpdateScheduleState(id string, change func(*state.ScheduleState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.st[id]
	before := s.LastError
	change(&s)
	m.st[id] = s
	if s.LastError != before { // a new result: the scheduler never reuses an ErrorRef
		m.history[id] = append(m.history[id], s)
	}
}

// skips returns the skip codes recorded for id, in order, as "code params".
func (m *memStore) skips(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, s := range m.history[id] {
		if s.LastResult == state.ResultSkipped && s.LastError != nil {
			p := s.LastError.Params
			out = append(out, string(s.LastError.Code)+" "+p["slot"]+p["count"]+" "+p["from"]+"|"+p["to"])
		}
	}
	return out
}

// fakeRunner reports every job; with block set, runs wait for a release (or
// the context) before returning.
type fakeRunner struct {
	jobs    chan Job
	block   bool
	release chan struct{}
}

func newRunner(block bool) *fakeRunner {
	return &fakeRunner{jobs: make(chan Job, 1000), block: block, release: make(chan struct{})}
}

func (r *fakeRunner) Run(ctx context.Context, job Job) Result {
	r.jobs <- job
	if r.block {
		select {
		case <-r.release:
		case <-ctx.Done():
			return Result{Status: state.ResultFailed, Err: apperr.New(apperr.Internal, ctx.Err())}
		}
	}
	return Result{Status: state.ResultOK}
}

type harness struct {
	t      *testing.T
	loc    *time.Location
	clk    *clock.Fake
	store  *memStore
	runner *fakeRunner
	s      *Scheduler
	logs   *bytes.Buffer
	next   []*NextRun
	mu     sync.Mutex
}

type harnessOpts struct {
	start  time.Time
	jitter time.Duration
	block  bool
	store  *memStore
}

func newHarness(t *testing.T, o harnessOpts, schedules ...config.Schedule) *harness {
	t.Helper()
	h := &harness{t: t, loc: rome(t), clk: clock.NewFake(o.start), store: o.store, runner: newRunner(o.block),
		logs: &bytes.Buffer{}}
	if h.store == nil {
		h.store = newMemStore()
	}
	h.s = New(Options{
		Clock: h.clk, Location: h.loc, Runner: h.runner, Store: h.store,
		Log:    slog.New(slog.NewTextHandler(syncWriter{&h.mu, h.logs}, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Jitter: func() time.Duration { return o.jitter },
		OnNext: func(n *NextRun) { h.mu.Lock(); h.next = append(h.next, n); h.mu.Unlock() },
	})
	h.s.Reload(schedules)
	t.Cleanup(func() {
		close(h.runner.release)
		h.s.Stop(time.Second)
	})
	return h
}

type syncWriter struct {
	mu *sync.Mutex
	b  *bytes.Buffer
}

func (w syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (h *harness) logText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.logs.String()
}

// at moves the clock, runs one step, waits for the runs it started and
// returns the slots of their jobs. With blocking runs it returns nothing:
// use waitJobs.
func (h *harness) at(t time.Time) []string {
	h.t.Helper()
	h.clk.Set(t)
	h.s.step()
	if h.runner.block {
		return nil
	}
	h.s.runs.Wait()
	return h.drain()
}

func (h *harness) drain() []string {
	var out []string
	for {
		select {
		case j := <-h.runner.jobs:
			out = append(out, j.ScheduleID+"@"+fmtSlot(j.Slot))
		default:
			return out
		}
	}
}

// waitJobs waits for n jobs from runs that block.
func (h *harness) waitJobs(n int) []string {
	h.t.Helper()
	var out []string
	for len(out) < n {
		select {
		case j := <-h.runner.jobs:
			out = append(out, j.ScheduleID+"@"+fmtSlot(j.Slot))
		case <-time.After(5 * time.Second):
			h.t.Fatalf("got %d jobs, want %d: %v", len(out), n, out)
		}
	}
	return out
}

func sched(id string, c config.Cadence) config.Schedule {
	return config.Schedule{ID: id, Name: "Report " + id, Enabled: true, Cadence: c}
}

func eq(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("%s:\n got  %v\n want %v", what, got, want)
	}
}

func june(h, m, s int) time.Time {
	return time.Date(2026, 6, 1, h, m, s, 0, mustRome())
}

func mustRome() *time.Location {
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		panic(err)
	}
	return loc
}

func TestFiresAtTheSlotAndRecordsState(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 30, 0)}, sched("a", every(1, 0, "", nil)))
	eq(t, "before", h.at(june(9, 59, 59)))
	eq(t, "at the slot", h.at(june(10, 0, 0)), "a@06-01 Mon 10:00 +02")
	eq(t, "same slot again", h.at(june(10, 0, 30)))
	st := h.store.ScheduleState("a")
	if !st.LastSlot.Equal(june(10, 0, 0)) || !st.LastRunAt.Equal(june(10, 0, 0)) || st.LastResult != state.ResultOK || st.LastError != nil {
		t.Fatalf("state: %+v", st)
	}
	if _, off := st.LastSlot.Zone(); off != 2*3600 {
		t.Fatalf("last_slot must keep the local offset, got %v", st.LastSlot)
	}
}

func TestJitterDelaysWithoutAccumulating(t *testing.T) {
	jit := 100 * time.Second
	h := newHarness(t, harnessOpts{start: june(9, 30, 0), jitter: jit}, sched("a", every(1, 0, "", nil)))
	eq(t, "slot, jitter pending", h.at(june(10, 1, 39)))
	eq(t, "slot + jitter", h.at(june(10, 1, 40)), "a@06-01 Mon 10:00 +02")
	eq(t, "next slot before its jitter", h.at(june(11, 1, 39)))
	eq(t, "next slot + jitter (not + 2·jitter)", h.at(june(11, 1, 40)), "a@06-01 Mon 11:00 +02")

	// The loop wakes at the due time, at most every MaxSleep.
	h.clk.Set(june(11, 30, 0))
	if d := h.s.step(); d != MaxSleep {
		t.Errorf("far from the slot: sleep %v, want %v", d, MaxSleep)
	}
	h.clk.Set(june(12, 1, 30))
	if d := h.s.step(); d != 10*time.Second {
		t.Errorf("10 s before slot + jitter: sleep %v", d)
	}
}

func TestDefaultJitterBounds(t *testing.T) {
	s := New(Options{Clock: clock.NewFake(june(0, 0, 0)), Location: time.UTC, Runner: newRunner(false), Store: newMemStore()})
	seen := map[bool]bool{}
	for range 2000 {
		j := s.o.Jitter()
		if j < 0 || j >= MaxJitter {
			t.Fatalf("jitter %v out of [0, %v)", j, MaxJitter)
		}
		seen[j < MaxJitter/2] = true
	}
	if len(seen) != 2 {
		t.Fatal("jitter doesn't look random")
	}
}

func TestToleranceBoundary(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 59, 0)}, sched("a", every(1, 0, "", nil)))
	eq(t, "exactly 5 minutes late", h.at(june(10, 5, 0)), "a@06-01 Mon 10:00 +02")

	h = newHarness(t, harnessOpts{start: june(9, 59, 0)}, sched("a", every(1, 0, "", nil)))
	eq(t, "5 minutes and 1 second late", h.at(june(10, 5, 1)))
	eq(t, "recorded", h.store.skips("a"), "slot.missed 1 2026-06-01 10:00|2026-06-01 10:00")
}

func TestMissedAfterSleepIsOneEntry(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(8, 0, 0)}, sched("a", every(0, 15, "", nil)), sched("b", fixed([]string{"09:00"})))
	eq(t, "wake at 11:07: nothing recovered", h.at(june(11, 7, 0)))
	eq(t, "a: one aggregated entry", h.store.skips("a"), "slot.missed 12 2026-06-01 08:15|2026-06-01 11:00")
	eq(t, "b: one entry", h.store.skips("b"), "slot.missed 1 2026-06-01 09:00|2026-06-01 09:00")
	if strings.Count(h.logText(), "code=slot.missed") != 2 {
		t.Errorf("want one log line per schedule:\n%s", h.logText())
	}
	eq(t, "the next slot runs normally", h.at(june(11, 15, 0)), "a@06-01 Mon 11:15 +02")
}

func TestMissedThenRunWithinTolerance(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(8, 0, 0)}, sched("a", every(0, 15, "", nil)))
	eq(t, "wake at 11:02", h.at(june(11, 2, 0)), "a@06-01 Mon 11:00 +02")
	eq(t, "missed before", h.store.skips("a"), "slot.missed 11 2026-06-01 08:15|2026-06-01 10:45")
	if st := h.store.ScheduleState("a"); st.LastResult != state.ResultOK || !st.LastSlot.Equal(june(11, 0, 0)) {
		t.Fatalf("the run's result comes last: %+v", st)
	}
}

func TestClockSetBackDoesNotRerun(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 59, 0)}, sched("a", every(1, 0, "", nil)))
	eq(t, "run", h.at(june(10, 0, 0)), "a@06-01 Mon 10:00 +02")
	// The schedule is edited back and forth (cursor restarts from now) after
	// the clock went back: the same slot never runs twice.
	h.clk.Set(june(9, 58, 0))
	h.s.Reload([]config.Schedule{sched("a", every(2, 0, "", nil))})
	h.s.Reload([]config.Schedule{sched("a", every(1, 0, "", nil))})
	eq(t, "same slot again", h.at(june(10, 0, 0)))
}

func TestStartupDoesNotReportTheClosedTime(t *testing.T) {
	store := newMemStore()
	store.st["a"] = state.ScheduleState{LastSlot: time.Date(2026, 5, 20, 10, 0, 0, 0, mustRome())}
	h := newHarness(t, harnessOpts{start: june(10, 30, 0), store: store}, sched("a", every(1, 0, "", nil)))
	eq(t, "first look", h.at(june(10, 30, 0)))
	eq(t, "nothing recorded", h.store.skips("a"))
	eq(t, "next slot", h.at(june(11, 0, 0)), "a@06-01 Mon 11:00 +02")
}

func TestPausedIsOneEntryPerStretch(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 30, 0)}, sched("a", every(1, 0, "", nil)))
	h.store.setActive(false)
	for _, hr := range []int{10, 11, 12} {
		eq(t, "paused", h.at(june(hr, 0, 0)))
	}
	eq(t, "one entry, updated", h.store.skips("a"),
		"slot.paused 1 2026-06-01 10:00|2026-06-01 10:00",
		"slot.paused 2 2026-06-01 10:00|2026-06-01 11:00",
		"slot.paused 3 2026-06-01 10:00|2026-06-01 12:00")
	if st := h.store.ScheduleState("a"); !st.LastSlot.IsZero() {
		t.Fatalf("a skip doesn't set last_slot: %+v", st)
	}
	h.store.setActive(true)
	eq(t, "resumed", h.at(june(12, 30, 0)))
	if n := strings.Count(h.logText(), "slots skipped while paused"); n != 1 {
		t.Errorf("want one log line for the stretch, got %d:\n%s", n, h.logText())
	}
	eq(t, "runs again", h.at(june(13, 0, 0)), "a@06-01 Mon 13:00 +02")

	h.store.setActive(false)
	h.at(june(14, 0, 0))
	eq(t, "a new stretch starts from 1", h.store.skips("a")[3:], "slot.paused 1 2026-06-01 14:00|2026-06-01 14:00")
}

func TestPausedThroughSleepCountsAsPaused(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 30, 0)}, sched("a", every(1, 0, "", nil)))
	h.store.setActive(false)
	h.at(june(13, 0, 0))
	eq(t, "paused, not missed", h.store.skips("a")[3:], "slot.paused 4 2026-06-01 10:00|2026-06-01 13:00")
}

func TestOverlapSkips(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 30, 0), block: true}, sched("a", every(1, 0, "", nil)))
	h.at(june(10, 0, 0))
	eq(t, "first run started", h.waitJobs(1), "a@06-01 Mon 10:00 +02")
	h.at(june(11, 0, 0))
	eq(t, "skipped", h.store.skips("a"), "slot.overlap 2026-06-01 11:00 |")
	h.runner.release <- struct{}{}
	h.s.runs.Wait()
	if st := h.store.ScheduleState("a"); st.LastResult != state.ResultOK || !st.LastSlot.Equal(june(10, 0, 0)) {
		t.Fatalf("state after the run: %+v", st)
	}
	h.at(june(12, 0, 0))
	eq(t, "free again", h.waitJobs(1), "a@06-01 Mon 12:00 +02")
}

func TestSchedulesRunInParallel(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 30, 0), block: true},
		sched("a", every(1, 0, "", nil)), sched("b", fixed([]string{"10:00"})))
	h.at(june(10, 0, 0))
	// Both runs are in progress at once: neither waits for the other.
	got := h.waitJobs(2)
	slices.Sort(got)
	eq(t, "both started", got, "a@06-01 Mon 10:00 +02", "b@06-01 Mon 10:00 +02")
}

func TestReload(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 30, 0), jitter: 100 * time.Second}, sched("a", every(1, 0, "", nil)))
	// A rename (same cadence) keeps the pending slot and its jitter.
	h.clk.Set(june(10, 1, 0))
	renamed := sched("a", every(1, 0, "", nil))
	renamed.Name = "Renamed"
	h.s.Reload([]config.Schedule{renamed})
	h.clk.Set(june(10, 1, 40))
	h.s.step()
	h.s.runs.Wait()
	select {
	case j := <-h.runner.jobs:
		if j.ScheduleName != "Renamed" || !j.Slot.Equal(june(10, 0, 0)) {
			t.Fatalf("job %+v", j)
		}
	default:
		t.Fatal("an unchanged cadence must keep its pending slot")
	}

	// An edit restarts from now: 10:15 has passed and is never fired.
	h.clk.Set(june(10, 30, 0))
	h.s.Reload([]config.Schedule{sched("a", fixed([]string{"10:15", "10:45"}))})
	eq(t, "no back-firing", h.at(june(10, 46, 40)), "a@06-01 Mon 10:45 +02")
	eq(t, "nothing reported", h.store.skips("a"))

	// Disabled, invalid and removed schedules don't run.
	off := sched("a", every(1, 0, "", nil))
	off.Enabled = false
	bad := sched("b", every(0, 0, "", nil))
	h.s.Reload([]config.Schedule{off, bad})
	eq(t, "disabled and invalid", h.at(june(11, 0, 0)))
	if !strings.Contains(h.logText(), "code=cadence.interval_invalid") {
		t.Errorf("an invalid cadence is logged:\n%s", h.logText())
	}
	h.mu.Lock()
	last := h.next[len(h.next)-1]
	h.mu.Unlock()
	if last != nil {
		t.Fatalf("no next run expected, got %+v", last)
	}
}

func TestNextRunIsPublished(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(9, 30, 0)},
		sched("a", every(1, 0, "", nil)), sched("b", fixed([]string{"09:45"})))
	h.at(june(9, 45, 0))
	h.at(june(9, 50, 0)) // unchanged: no new call
	h.mu.Lock()
	defer h.mu.Unlock()
	var got []string
	for _, n := range h.next {
		got = append(got, n.ScheduleName+" "+fmtSlot(n.At))
	}
	eq(t, "next runs", got, "Report b 06-01 Mon 09:45 +02", "Report a 06-01 Mon 10:00 +02")
}

func TestDSTSpringGap(t *testing.T) {
	loc := mustRome()
	at := func(h, m int) time.Time { return time.Date(2026, 3, 29, h, m, 0, 0, loc) }
	h := newHarness(t, harnessOpts{start: at(1, 45)}, sched("a", every(0, 30, "", win("01:00", "04:00"))))
	eq(t, "03:00 CEST: the 02:00 gap and the 03:00 run", h.at(at(3, 0)), "a@03-29 Sun 03:00 +02")
	eq(t, "03:30", h.at(at(3, 30)), "a@03-29 Sun 03:30 +02")
	eq(t, "gaps recorded", h.store.skips("a"), "slot.dst_gap 2026-03-29 02:00 |", "slot.dst_gap 2026-03-29 02:30 |")
}

func TestDSTAutumnRepeat(t *testing.T) {
	utc := func(h, m int) time.Time { return time.Date(2026, 10, 25, h, m, 0, 0, time.UTC) }
	h := newHarness(t, harnessOpts{start: utc(23, 40).AddDate(0, 0, -1)}, sched("a", every(0, 30, "", nil)))
	var got []string
	for i := 0; i <= 10; i++ { // every 15 minutes from 23:45Z (01:45 CEST) to 02:15Z (03:15 CET)
		got = append(got, h.at(utc(23, 45).AddDate(0, 0, -1).Add(time.Duration(i)*15*time.Minute))...)
	}
	eq(t, "runs", got, "a@10-25 Sun 02:00 +02", "a@10-25 Sun 02:30 +02", "a@10-25 Sun 03:00 +01")
	eq(t, "second occurrences", h.store.skips("a"), "slot.dst_repeat 2026-10-25 02:00 |", "slot.dst_repeat 2026-10-25 02:30 |")
}

func TestDSTRestartAfterFirstAutumnOccurrence(t *testing.T) {
	utc := func(h, m int) time.Time { return time.Date(2026, 10, 25, h, m, 0, 0, time.UTC) }
	for _, start := range []time.Time{utc(0, 45), utc(1, 20)} { // 02:45 CEST (first pass), 02:20 CET (second pass)
		store := newMemStore()
		// The 02:30 slot ran at its first occurrence (+02:00), then the app restarted.
		store.st["a"] = state.ScheduleState{LastSlot: utc(0, 30).In(mustRome())}
		h := newHarness(t, harnessOpts{start: start, store: store}, sched("a", every(0, 30, "", nil)))
		var got []string
		for m := start; !m.After(utc(2, 0)); m = m.Add(5 * time.Minute) {
			got = append(got, h.at(m)...)
		}
		eq(t, "after a restart at "+start.Format("15:04Z"), got, "a@10-25 Sun 03:00 +01")
		skips := h.store.skips("a")
		if len(skips) == 0 || skips[len(skips)-1] != "slot.dst_repeat 2026-10-25 02:30 |" {
			t.Errorf("start %s: skips %v", start.Format("15:04Z"), skips)
		}
	}
}

func TestLoopAndStop(t *testing.T) {
	clk := clock.NewFake(june(9, 59, 0))
	runner := newRunner(true)
	tick := make(chan time.Time)
	s := New(Options{Clock: clk, Location: mustRome(), Runner: runner, Store: newMemStore(),
		Jitter: func() time.Duration { return 0 },
		After:  func(time.Duration) <-chan time.Time { return tick }})
	s.Reload([]config.Schedule{sched("a", every(1, 0, "", nil))})
	s.Start()
	clk.Set(june(10, 0, 0))
	tick <- time.Time{} // the timer fires: the loop looks at the clock
	select {
	case j := <-runner.jobs:
		if !j.Slot.Equal(june(10, 0, 0)) {
			t.Fatalf("job %+v", j)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the loop didn't run the slot")
	}
	// Stop cancels the run in progress (it waits on its context) and the loop.
	if !s.Stop(5 * time.Second) {
		t.Fatal("the run didn't end after Stop")
	}
}

func TestRunNow(t *testing.T) {
	h := newHarness(t, harnessOpts{start: june(10, 3, 0), block: true})
	if err := h.s.RunNow("a", "Report a"); err != nil {
		t.Fatal(err)
	}
	job := <-h.runner.jobs
	if !job.Manual || job.ScheduleID != "a" || !job.Slot.Equal(june(10, 3, 0)) || !job.Deadline.IsZero() {
		t.Fatalf("job %+v", job)
	}
	// Busy while the first one runs.
	err := h.s.RunNow("a", "Report a")
	if e := apperr.As(err); e.Code != apperr.ScheduleBusy || e.Params["name"] != "Report a" {
		t.Fatalf("second: %v", err)
	}
	st := h.store.ScheduleState("a")
	if !st.LastSlot.IsZero() || !st.LastRunAt.Equal(june(10, 3, 0)) {
		t.Fatalf("state %+v", st)
	}
	h.runner.release <- struct{}{}
	deadline := time.Now().Add(3 * time.Second)
	for h.s.RunNow("a", "Report a") != nil {
		if time.Now().After(deadline) {
			t.Fatal("still busy after the run ended")
		}
		time.Sleep(time.Millisecond)
	}
	<-h.runner.jobs
}

type problemRunner struct{}

func (problemRunner) Run(context.Context, Job) Result {
	p := apperr.New(apperr.FileNotFound, nil).With("path", `C:\x`)
	return Result{Status: state.ResultPartial, Err: p, Problems: []*apperr.Error{p, apperr.New(apperr.SendFailed, nil)}}
}

func TestRunStoresProblems(t *testing.T) {
	store := newMemStore()
	s := New(Options{Clock: clock.NewFake(june(10, 0, 0)), Location: time.UTC, Runner: problemRunner{}, Store: store})
	if err := s.RunNow("a", "A"); err != nil {
		t.Fatal(err)
	}
	s.Stop(time.Second)
	st := store.ScheduleState("a")
	if st.LastResult != state.ResultPartial || st.LastError.Code != apperr.FileNotFound || len(st.Problems) != 2 ||
		st.Problems[1].Code != apperr.SendFailed || st.Problems[1].Params == nil {
		t.Fatalf("state %+v", st)
	}
	if err := s.RunNow("a", "A"); apperr.As(err).Code != apperr.ScheduleBusy {
		t.Fatalf("after stop: %v", err)
	}
}
