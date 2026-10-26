package scheduler

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/state"
)

const (
	// Tolerance is how late a slot may still start.
	Tolerance = 5 * time.Minute
	// MaxJitter bounds the random start delay of a run.
	MaxJitter = 2 * time.Minute
	// MaxSleep is the longest the loop waits before looking at the clock again.
	MaxSleep = 30 * time.Second
)

// Store is what the scheduler needs from the app state (app.Store).
type Store interface {
	// RoutineActive is false while the routine is paused from the tray.
	RoutineActive() bool
	ScheduleState(id string) state.ScheduleState
	// UpdateScheduleState changes and persists a schedule's state.
	UpdateScheduleState(id string, change func(*state.ScheduleState))
}

// NextRun is the earliest upcoming run among all schedules.
type NextRun struct {
	ScheduleID   string
	ScheduleName string
	At           time.Time
}

// Options are the scheduler's inputs. Clock, Location, Runner and Store are required.
type Options struct {
	Clock    clock.Clock
	Location *time.Location
	Runner   Runner
	Store    Store
	Log      *slog.Logger
	// Jitter draws a run's start delay; nil is uniform in [0, MaxJitter).
	Jitter func() time.Duration
	// OnNext is called (outside the scheduler's lock) when the next run
	// changes; nil when there is none.
	OnNext func(*NextRun)
	// After is the loop's timer; nil is time.After. Tests replace it.
	After func(time.Duration) <-chan time.Time
}

// Scheduler fires the slots of the enabled schedules. See the package doc.
type Scheduler struct {
	o Options

	mu       sync.Mutex
	notifyMu sync.Mutex // keeps OnNext calls in the order of the changes
	entries  map[string]*entry
	busy     map[string]bool // schedule id → a run is in progress
	next     *NextRun        // last value given to OnNext
	started  bool

	ctx      context.Context
	cancel   context.CancelFunc
	wake     chan struct{}
	loopDone chan struct{}
	runs     sync.WaitGroup
}

// entry is one enabled schedule with a valid cadence.
type entry struct {
	id, name string
	cadence  config.Cadence
	plan     plan
	cur      *cursor
	paused   stretch
}

// stretch aggregates skipped slots (missed, or paused) into one report.
type stretch struct {
	count    int
	from, to time.Time // wall clock of the first and last slot
}

func (s *stretch) add(wall time.Time) {
	if s.count == 0 {
		s.from = wall
	}
	s.to = wall
	s.count++
}

func (s *stretch) params(name string) map[string]string {
	return map[string]string{"schedule": name, "count": strconv.Itoa(s.count),
		"from": slotText(s.from), "to": slotText(s.to)}
}

// New builds a scheduler with no schedules: call Reload, then Start.
func New(o Options) *Scheduler {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.Jitter == nil {
		o.Jitter = func() time.Duration { return rand.N(MaxJitter) }
	}
	if o.After == nil {
		o.After = time.After
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		o:        o,
		entries:  map[string]*entry{},
		busy:     map[string]bool{},
		ctx:      ctx,
		cancel:   cancel,
		wake:     make(chan struct{}, 1),
		loopDone: make(chan struct{}),
	}
}

// Reload replaces the schedules (the whole config list; disabled ones and
// those with an invalid cadence are ignored, the latter with a warning). A
// schedule whose cadence is unchanged keeps its place; a new or edited one
// starts from now, so nothing before now is run or reported. A schedule's
// in-progress run is unaffected (and still blocks an overlapping run).
func (s *Scheduler) Reload(schedules []config.Schedule) {
	s.mu.Lock()
	now := s.o.Clock.Now()
	keep := map[string]*entry{}
	for _, sc := range schedules {
		if !sc.Enabled {
			continue
		}
		p, err := compile(sc.Cadence)
		if err != nil {
			s.o.Log.Warn("scheduler: schedule ignored, invalid cadence", "schedule", sc.ID, "name", sc.Name,
				"code", apperr.As(err).Code, "err", err)
			continue
		}
		if old := s.entries[sc.ID]; old != nil && old.plan.equal(&p) {
			old.name, old.cadence = sc.Name, sc.Cadence
			keep[sc.ID] = old
			continue
		}
		keep[sc.ID] = &entry{id: sc.ID, name: sc.Name, cadence: sc.Cadence, plan: p, cur: newCursor(&p, s.o.Location, now)}
	}
	for id, old := range s.entries {
		if keep[id] != old {
			s.closePaused(old)
		}
	}
	// The cursors point at their entry's plan: re-point the new ones (the
	// loop above built them on a local copy).
	for _, e := range keep {
		e.cur.p = &e.plan
	}
	s.entries = keep
	s.o.Log.Info("scheduler: schedules loaded", "active", len(keep), "configured", len(schedules))
	s.publishUnlock()
	s.Wake()
}

// Start runs the loop on its own goroutine until Stop.
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	go s.loop()
}

// Wake makes the loop look at the clock now (e.g. after a config change).
func (s *Scheduler) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Stop ends the loop, cancels the context of the runs in progress and waits
// up to timeout for them; it reports whether they all finished.
func (s *Scheduler) Stop(timeout time.Duration) bool {
	s.cancel()
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if started {
		<-s.loopDone
	}
	s.mu.Lock()
	for _, e := range s.entries {
		s.closePaused(e)
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.runs.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		s.o.Log.Warn("scheduler: runs still in progress at stop", "timeout", timeout)
		return false
	}
}

func (s *Scheduler) loop() {
	defer close(s.loopDone)
	for {
		d := s.step()
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		case <-s.o.After(d):
		}
	}
}

// step handles every event that is due now and returns how long to wait
// before the next look: until the next due event, at most MaxSleep, so a
// clock change or a resume from sleep is noticed within MaxSleep.
func (s *Scheduler) step() time.Duration {
	s.mu.Lock()
	now := s.o.Clock.Now()
	active := s.o.Store.RoutineActive()
	var wake time.Time
	ids := make([]string, 0, len(s.entries))
	for id := range s.entries {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		e := s.entries[id]
		if active {
			s.closePaused(e)
		}
		if due, ok := s.stepEntry(e, now, active); ok && (wake.IsZero() || due.Before(wake)) {
			wake = due
		}
	}
	s.publishUnlock()
	if wake.IsZero() {
		return MaxSleep
	}
	return min(max(wake.Sub(now), 0), MaxSleep)
}

// stepEntry handles the due events of one schedule; it returns when its next
// event is due (ok false if it has none). Called with s.mu held.
func (s *Scheduler) stepEntry(e *entry, now time.Time, active bool) (time.Time, bool) {
	var missed stretch
	defer s.flushMissed(e, &missed)
	for {
		ev := e.cur.peek()
		if ev == nil {
			return time.Time{}, false
		}
		if ev.kind == fire && !ev.jitterSet {
			ev.jitter, ev.jitterSet = s.o.Jitter(), true
		}
		if now.Before(ev.due()) {
			return ev.due(), true
		}
		cur := *ev
		e.cur.pop()
		late := now.Sub(cur.at) > Tolerance
		switch {
		case cur.kind != fire:
			// A DST gap or repeat is only worth reporting when it happens
			// live; while paused or asleep it was never going to run.
			if late || !active {
				continue
			}
			code := apperr.SlotDSTGap
			if cur.kind == repeat {
				code = apperr.SlotDSTRepeat
			}
			s.skip(e, code, cur.wall)
		case !active:
			e.paused.add(cur.wall)
			s.record(e, apperr.SlotPaused, e.paused.params(e.name), false)
		case late:
			missed.add(cur.wall)
		default:
			s.flushMissed(e, &missed)
			s.fire(e, cur, now)
		}
	}
}

// fire starts the run of a slot unless it already ran (same wall-clock slot
// in last_slot: a restart, or the second autumn occurrence) or the
// schedule's previous run is still going. Called with s.mu held.
func (s *Scheduler) fire(e *entry, ev event, now time.Time) {
	last := s.o.Store.ScheduleState(e.id).LastSlot
	if !last.IsZero() && sameWall(last.In(s.o.Location), ev.wall) {
		if last.Equal(ev.at) {
			s.o.Log.Debug("scheduler: slot already ran", "schedule", e.id, "slot", slotText(ev.wall))
			return
		}
		s.skip(e, apperr.SlotDSTRepeat, ev.wall)
		return
	}
	if s.busy[e.id] {
		s.skip(e, apperr.SlotOverlap, ev.wall)
		return
	}
	s.busy[e.id] = true
	s.o.Store.UpdateScheduleState(e.id, func(st *state.ScheduleState) {
		st.LastSlot = ev.at
		st.LastRunAt = now
	})
	job := Job{ScheduleID: e.id, ScheduleName: e.name, Slot: ev.at, Deadline: ev.at.Add(Tolerance)}
	s.o.Log.Info("scheduler: run", "schedule", e.id, "name", e.name, "slot", ev.at.Format(time.RFC3339),
		"delay", now.Sub(ev.at).Round(time.Second))
	s.runs.Add(1)
	go func() {
		defer s.runs.Done()
		res := s.o.Runner.Run(s.ctx, job)
		s.o.Store.UpdateScheduleState(job.ScheduleID, func(st *state.ScheduleState) {
			st.LastResult = res.Status
			st.LastError = errorRef(res.Err)
		})
		if res.Err != nil {
			s.o.Log.Info("scheduler: run result", "schedule", job.ScheduleID, "status", res.Status,
				"code", res.Err.Code, "params", res.Err.Params, "err", res.Err)
		}
		s.mu.Lock()
		delete(s.busy, job.ScheduleID)
		s.mu.Unlock()
	}()
}

// skip records a single-slot skip reason in the log and the schedule's state.
func (s *Scheduler) skip(e *entry, code apperr.Code, wall time.Time) {
	s.record(e, code, map[string]string{"schedule": e.name, "slot": slotText(wall)}, true)
}

// record sets a skip reason as the schedule's last result.
func (s *Scheduler) record(e *entry, code apperr.Code, params map[string]string, log bool) {
	if log {
		s.o.Log.Info("scheduler: slot skipped", "schedule", e.id, "code", code, "params", params)
	} else {
		s.o.Log.Debug("scheduler: slot skipped", "schedule", e.id, "code", code, "params", params)
	}
	s.o.Store.UpdateScheduleState(e.id, func(st *state.ScheduleState) {
		st.LastResult = state.ResultSkipped
		st.LastError = &state.ErrorRef{Code: code, Params: params}
	})
}

// flushMissed reports the missed slots found in one look as one entry.
func (s *Scheduler) flushMissed(e *entry, m *stretch) {
	if m.count > 0 {
		s.record(e, apperr.SlotMissed, m.params(e.name), true)
		*m = stretch{}
	}
}

// closePaused logs the end of a paused stretch (state already holds it).
func (s *Scheduler) closePaused(e *entry) {
	if e.paused.count > 0 {
		s.o.Log.Info("scheduler: slots skipped while paused", "schedule", e.id, "code", apperr.SlotPaused,
			"params", e.paused.params(e.name))
		e.paused = stretch{}
	}
}

// nextRunLocked is the earliest upcoming run (ties: lowest schedule id).
func (s *Scheduler) nextRunLocked() *NextRun {
	var best *NextRun
	for _, e := range s.entries {
		ev := e.cur.nextFire()
		if ev == nil {
			continue
		}
		if best == nil || ev.at.Before(best.At) || (ev.at.Equal(best.At) && e.id < best.ScheduleID) {
			best = &NextRun{ScheduleID: e.id, ScheduleName: e.name, At: ev.at}
		}
	}
	return best
}

// publishUnlock recomputes the next run, releases s.mu (held by the caller)
// and calls OnNext if the next run changed, outside s.mu but in order.
func (s *Scheduler) publishUnlock() {
	n := s.nextRunLocked()
	same := (n == nil && s.next == nil) ||
		(n != nil && s.next != nil && n.ScheduleID == s.next.ScheduleID && n.ScheduleName == s.next.ScheduleName && n.At.Equal(s.next.At))
	s.next = n
	s.notifyMu.Lock()
	s.mu.Unlock()
	defer s.notifyMu.Unlock()
	if !same && s.o.OnNext != nil {
		s.o.OnNext(n)
	}
}

func errorRef(e *apperr.Error) *state.ErrorRef {
	if e == nil {
		return nil
	}
	p := e.Params
	if p == nil {
		p = map[string]string{}
	}
	return &state.ErrorRef{Code: e.Code, Params: p}
}
