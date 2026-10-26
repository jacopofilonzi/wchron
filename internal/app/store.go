// Package app holds the runtime state shared by the window and the tray, and
// the Wails service that exposes it to the frontend.
//
// Store is the single source of truth: every change (from a binding or from
// the tray) goes through it and is pushed to all subscribers as a full
// Snapshot, so the tray and the window always show the same state. main
// subscribes one listener that emits EventState to the frontend; the tray
// subscribes another. Service is the thin binding layer over the Store: every
// exported method on it becomes a frontend binding, so helpers are package
// functions or Store methods.
//
// Persistence: the Store starts from the config.json and state.json loaded
// by package startup and owns them afterwards. Theme, language and the
// keep-awake default are settings (config.json): if saving fails the change
// is rolled back and the caller gets config.write_failed. The routine pause
// is state (state.json): a failed save is only logged (state.write_failed)
// and the change still applies, since the routine must keep obeying the tray.
// The live keep-awake switch is not persisted (it starts from
// keep_awake_default at every launch, see Start), and autostart is read from
// and written to the OS only.
//
// The Store is also the scheduler's view of the app (scheduler.Store): the
// routine flag, each schedule's last slot/result in state.json (a failed save
// is only logged, like the pause), and NextRun, the earliest upcoming run the
// scheduler reports, which is part of the Snapshot (tray status line,
// EventState).
package app

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/platform"
	"github.com/jacopofilonzi/wchron/internal/state"
	"github.com/jacopofilonzi/wchron/internal/version"
)

// ChannelsState sums up the delivery channels (WhatsApp, later Telegram,
// ntfy…) for the sidebar and the tray: the worst state among the configured ones.
type ChannelsState string

const (
	// ChannelsNone: no channel configured yet (the only state until the
	// providers land, milestone 5).
	ChannelsNone         ChannelsState = "none"
	ChannelsConnected    ChannelsState = "connected"
	ChannelsDisconnected ChannelsState = "disconnected"
)

// Snapshot is the whole runtime state, sent to the frontend on every change.
type Snapshot struct {
	RoutineActive bool `json:"routine_active"`
	// KeepAwake is the live switch; KeepAwakeDefault is the setting applied at start.
	KeepAwake        bool          `json:"keep_awake"`
	KeepAwakeDefault bool          `json:"keep_awake_default"`
	Autostart        bool          `json:"autostart"`
	Theme            config.Theme  `json:"theme"`
	Language         i18n.Lang     `json:"language"`
	Channels         ChannelsState `json:"channels"`
	Version          string        `json:"version"`
	// NextRun is the earliest upcoming scheduled run; null when no enabled
	// schedule has one. It is shown even while the routine is paused.
	NextRun *NextRun `json:"next_run"`
}

// NextRun is a schedule's next slot (without jitter), in local time.
type NextRun struct {
	ScheduleID   string    `json:"schedule_id"`
	ScheduleName string    `json:"schedule_name"`
	At           time.Time `json:"at"`
}

// Deps are the Store's inputs.
type Deps struct {
	Inhibitor platform.SleepInhibitor
	Autostart platform.Autostart
	// AutostartOn is the value read from the OS at start.
	AutostartOn bool
	Config      config.Config
	State       state.State
	// SaveConfig and SaveState write the files (config.Save / state.Save on
	// the data dir paths); nil means "don't persist" (tests).
	SaveConfig func(config.Config) error
	SaveState  func(state.State) error
	Log        *slog.Logger
}

// Store owns the state and its side effects (keep-awake, autostart, files).
type Store struct {
	mu        sync.Mutex
	notifyMu  sync.Mutex // keeps listener calls in the order of the changes
	s         Snapshot
	cfg       config.Config
	st        state.State
	d         Deps
	listeners []func(Snapshot)
}

// NewStore builds the state from the loaded data. It has no side effects:
// Start applies the keep-awake default.
func NewStore(d Deps) *Store {
	if d.SaveConfig == nil {
		d.SaveConfig = func(config.Config) error { return nil }
	}
	if d.SaveState == nil {
		d.SaveState = func(state.State) error { return nil }
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	set := d.Config.Settings
	return &Store{
		s: Snapshot{
			RoutineActive:    d.State.RoutineActive,
			KeepAwakeDefault: set.KeepAwakeDefault,
			Autostart:        d.AutostartOn,
			Theme:            set.Theme,
			Language:         set.Language,
			Channels:         ChannelsNone,
			Version:          version.Version,
		},
		cfg: d.Config,
		st:  d.State,
		d:   d,
	}
}

// Start turns keep-awake on when the setting asks for it.
func (st *Store) Start() error {
	if !st.Snapshot().KeepAwakeDefault {
		return nil
	}
	_, err := st.SetKeepAwake(true)
	return err
}

// Snapshot returns the current state.
func (st *Store) Snapshot() Snapshot {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s
}

// Subscribe registers fn to receive every new state. Listeners run on the
// caller's goroutine, in order, and must not call back into the Store.
func (st *Store) Subscribe(fn func(Snapshot)) {
	st.notifyMu.Lock()
	defer st.notifyMu.Unlock()
	st.listeners = append(st.listeners, fn)
}

// update applies mutate under the lock; if it reports a change, every listener
// gets the new snapshot. mutate returning an error leaves the state untouched.
func (st *Store) update(mutate func(s *Snapshot) (bool, error)) (Snapshot, error) {
	st.mu.Lock()
	next := st.s
	changed, err := mutate(&next)
	if err != nil || !changed {
		cur := st.s
		st.mu.Unlock()
		return cur, err
	}
	st.s = next
	st.notifyMu.Lock()
	st.mu.Unlock()
	defer st.notifyMu.Unlock()
	for _, l := range st.listeners {
		l(next)
	}
	return next, nil
}

// saveSettings persists a settings change; called under st.mu. On failure
// the in-memory config is unchanged, so the caller's change is rolled back.
func (st *Store) saveSettings(change func(*config.Settings)) error {
	next := st.cfg
	change(&next.Settings)
	if err := st.d.SaveConfig(next); err != nil {
		st.d.Log.Error("save settings", "err", err)
		e := apperr.As(err)
		if e.Code == apperr.Internal {
			e = apperr.New(apperr.ConfigWriteFailed, err)
		}
		return e
	}
	st.cfg = next
	return nil
}

// SetRoutineActive pauses or resumes the routine (no scheduler yet: state
// only). The pause survives a restart; a failed save is only logged.
func (st *Store) SetRoutineActive(on bool) (Snapshot, error) {
	return st.update(func(s *Snapshot) (bool, error) {
		if s.RoutineActive == on {
			return false, nil
		}
		next := st.st
		next.RoutineActive = on
		if err := st.d.SaveState(next); err != nil {
			st.d.Log.Warn("save routine state", "err", err)
		}
		st.st = next
		s.RoutineActive = on
		return true, nil
	})
}

// SetKeepAwake turns the sleep inhibitor on or off; on failure the state stays.
func (st *Store) SetKeepAwake(on bool) (Snapshot, error) {
	return st.update(func(s *Snapshot) (bool, error) {
		if s.KeepAwake == on {
			return false, nil
		}
		var err error
		if on {
			err = st.d.Inhibitor.Enable()
		} else {
			err = st.d.Inhibitor.Disable()
		}
		if err != nil {
			return false, apperr.New(apperr.KeepAwakeFailed, fmt.Errorf("set keep awake %v: %w", on, err))
		}
		s.KeepAwake = on
		return true, nil
	})
}

// SetKeepAwakeDefault sets whether keep-awake is turned on at every start.
// It doesn't change the live switch.
func (st *Store) SetKeepAwakeDefault(on bool) (Snapshot, error) {
	return st.update(func(s *Snapshot) (bool, error) {
		if s.KeepAwakeDefault == on {
			return false, nil
		}
		if err := st.saveSettings(func(c *config.Settings) { c.KeepAwakeDefault = on }); err != nil {
			return false, err
		}
		s.KeepAwakeDefault = on
		return true, nil
	})
}

// SetAutostart writes or removes the login start entry; on failure the state stays.
func (st *Store) SetAutostart(on bool) (Snapshot, error) {
	return st.update(func(s *Snapshot) (bool, error) {
		if s.Autostart == on {
			return false, nil
		}
		var err error
		if on {
			err = st.d.Autostart.Enable()
		} else {
			err = st.d.Autostart.Disable()
		}
		if err != nil {
			return false, apperr.New(apperr.AutostartFailed, fmt.Errorf("set autostart %v: %w", on, err))
		}
		s.Autostart = on
		return true, nil
	})
}

// SetTheme changes the UI theme. An unknown value can only come from a bug.
func (st *Store) SetTheme(t config.Theme) (Snapshot, error) {
	return st.update(func(s *Snapshot) (bool, error) {
		if !slices.Contains(config.Themes, t) {
			return false, apperr.New(apperr.Internal, fmt.Errorf("unknown theme %q", t))
		}
		if s.Theme == t {
			return false, nil
		}
		if err := st.saveSettings(func(c *config.Settings) { c.Theme = t }); err != nil {
			return false, err
		}
		s.Theme = t
		return true, nil
	})
}

// SetLanguage changes the UI and tray language.
func (st *Store) SetLanguage(l i18n.Lang) (Snapshot, error) {
	return st.update(func(s *Snapshot) (bool, error) {
		if !i18n.Valid(l) {
			return false, apperr.New(apperr.Internal, fmt.Errorf("unknown language %q", l))
		}
		if s.Language == l {
			return false, nil
		}
		if err := st.saveSettings(func(c *config.Settings) { c.Language = l }); err != nil {
			return false, err
		}
		s.Language = l
		return true, nil
	})
}

// Schedules returns the configured schedules (for the scheduler's Reload).
func (st *Store) Schedules() []config.Schedule {
	st.mu.Lock()
	defer st.mu.Unlock()
	return slices.Clone(st.cfg.Schedules)
}

// RoutineActive is false while the routine is paused.
func (st *Store) RoutineActive() bool { return st.Snapshot().RoutineActive }

// ScheduleState returns a schedule's last-run state (zero if it never ran).
func (st *Store) ScheduleState(id string) state.ScheduleState {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.st.Schedules[id]
}

// UpdateScheduleState changes a schedule's last-run state and saves
// state.json; a failed save is only logged (state.write_failed), the change
// stays in memory.
func (st *Store) UpdateScheduleState(id string, change func(*state.ScheduleState)) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.st.Schedules == nil {
		st.st.Schedules = map[string]state.ScheduleState{}
	}
	s := st.st.Schedules[id]
	change(&s)
	st.st.Schedules[id] = s
	if err := st.d.SaveState(st.st); err != nil {
		st.d.Log.Warn("save schedule state", "schedule", id, "err", err)
	}
}

// SetNextRun records the next scheduled run (nil for none) and notifies the
// listeners when it changed.
func (st *Store) SetNextRun(n *NextRun) {
	_, _ = st.update(func(s *Snapshot) (bool, error) {
		if sameNextRun(s.NextRun, n) {
			return false, nil
		}
		s.NextRun = n
		return true, nil
	})
}

func sameNextRun(a, b *NextRun) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ScheduleID == b.ScheduleID && a.ScheduleName == b.ScheduleName && a.At.Equal(b.At)
}

// Shutdown releases keep-awake. Windows drops it anyway when the process
// exits; releasing it explicitly keeps the quit path independent of that.
func (st *Store) Shutdown() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.s.KeepAwake {
		if err := st.d.Inhibitor.Disable(); err != nil {
			return fmt.Errorf("release keep awake: %w", err)
		}
	}
	return nil
}
