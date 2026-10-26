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
// Milestone 1: the state lives in memory only (no config.json yet), so theme,
// language and the routine toggle reset at every start.
package app

import (
	"fmt"
	"sync"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/platform"
	"github.com/jacopofilonzi/wchron/internal/version"
)

// Theme is the UI colour scheme.
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
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
	RoutineActive bool      `json:"routine_active"`
	KeepAwake     bool      `json:"keep_awake"`
	Autostart     bool      `json:"autostart"`
	Theme         Theme     `json:"theme"`
	Language      i18n.Lang `json:"language"`
	Channels      ChannelsState `json:"channels"`
	Version       string    `json:"version"`
}

// Store owns the state and its side effects (keep-awake, autostart).
type Store struct {
	mu        sync.Mutex
	notifyMu  sync.Mutex // keeps listener calls in the order of the changes
	s         Snapshot
	inhibitor platform.SleepInhibitor
	autostart platform.Autostart
	listeners []func(Snapshot)
}

// NewStore returns the default state; autostartOn is the value read from the OS.
func NewStore(inh platform.SleepInhibitor, auto platform.Autostart, autostartOn bool) *Store {
	return &Store{
		s: Snapshot{
			RoutineActive: true,
			Autostart:     autostartOn,
			Theme:         ThemeSystem,
			Language:      i18n.Default,
			Channels:      ChannelsNone,
			Version:       version.Version,
		},
		inhibitor: inh,
		autostart: auto,
	}
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

// SetRoutineActive pauses or resumes the routine (no scheduler yet: state only).
func (st *Store) SetRoutineActive(on bool) (Snapshot, error) {
	return st.update(func(s *Snapshot) (bool, error) {
		if s.RoutineActive == on {
			return false, nil
		}
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
			err = st.inhibitor.Enable()
		} else {
			err = st.inhibitor.Disable()
		}
		if err != nil {
			return false, apperr.New(apperr.KeepAwakeFailed, fmt.Errorf("set keep awake %v: %w", on, err))
		}
		s.KeepAwake = on
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
			err = st.autostart.Enable()
		} else {
			err = st.autostart.Disable()
		}
		if err != nil {
			return false, apperr.New(apperr.AutostartFailed, fmt.Errorf("set autostart %v: %w", on, err))
		}
		s.Autostart = on
		return true, nil
	})
}

// SetTheme changes the UI theme. An unknown value can only come from a bug.
func (st *Store) SetTheme(t Theme) (Snapshot, error) {
	return st.update(func(s *Snapshot) (bool, error) {
		switch t {
		case ThemeSystem, ThemeLight, ThemeDark:
		default:
			return false, apperr.New(apperr.Internal, fmt.Errorf("unknown theme %q", t))
		}
		if s.Theme == t {
			return false, nil
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
		s.Language = l
		return true, nil
	})
}

// Shutdown releases keep-awake. Windows drops it anyway when the process
// exits; releasing it explicitly keeps the quit path independent of that.
func (st *Store) Shutdown() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.s.KeepAwake {
		if err := st.inhibitor.Disable(); err != nil {
			return fmt.Errorf("release keep awake: %w", err)
		}
	}
	return nil
}
