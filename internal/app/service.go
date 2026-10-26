package app

import (
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventState carries the full Snapshot after every change (backend → frontend).
const EventState = "app:state"

func init() {
	application.RegisterEvent[Snapshot](EventState)
}

// Service is the Wails service bound to the frontend. Each method returns the
// new state, so the caller doesn't have to wait for EventState.
type Service struct {
	store *Store
}

// NewService wraps the store for the bindings.
func NewService(st *Store) *Service { return &Service{store: st} }

// State returns the current state.
func (s *Service) State() Snapshot { return s.store.Snapshot() }

// SetRoutineActive pauses or resumes the routine.
func (s *Service) SetRoutineActive(on bool) (Snapshot, error) { return s.store.SetRoutineActive(on) }

// SetKeepAwake turns keep-awake on or off (keepawake.failed on error).
func (s *Service) SetKeepAwake(on bool) (Snapshot, error) { return s.store.SetKeepAwake(on) }

// SetAutostart turns start with Windows on or off (autostart.failed on error).
func (s *Service) SetAutostart(on bool) (Snapshot, error) { return s.store.SetAutostart(on) }

// SetTheme sets the theme: "system", "light" or "dark".
func (s *Service) SetTheme(t Theme) (Snapshot, error) { return s.store.SetTheme(t) }

// SetLanguage sets the language: "it" or "en".
func (s *Service) SetLanguage(l i18n.Lang) (Snapshot, error) { return s.store.SetLanguage(l) }
