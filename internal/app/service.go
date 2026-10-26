package app

import (
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/tokens"
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
	store   *Store
	logsDir string
	clock   clock.Clock
}

// NewService wraps the store for the bindings; logsDir is where Logs reads,
// clk the source of "now" for the previews (clock.Real in the app).
func NewService(st *Store, logsDir string, clk clock.Clock) *Service {
	return &Service{store: st, logsDir: logsDir, clock: clk}
}

// State returns the current state.
func (s *Service) State() Snapshot { return s.store.Snapshot() }

// SetRoutineActive pauses or resumes the routine.
func (s *Service) SetRoutineActive(on bool) (Snapshot, error) { return s.store.SetRoutineActive(on) }

// SetKeepAwake turns keep-awake on or off (keepawake.failed on error).
func (s *Service) SetKeepAwake(on bool) (Snapshot, error) { return s.store.SetKeepAwake(on) }

// SetKeepAwakeDefault sets whether keep-awake turns on at every start
// (config.write_failed on error).
func (s *Service) SetKeepAwakeDefault(on bool) (Snapshot, error) {
	return s.store.SetKeepAwakeDefault(on)
}

// SetAutostart turns start with Windows on or off (autostart.failed on error).
func (s *Service) SetAutostart(on bool) (Snapshot, error) { return s.store.SetAutostart(on) }

// SetTheme sets the theme: "system", "light" or "dark" (config.write_failed on error).
func (s *Service) SetTheme(t config.Theme) (Snapshot, error) { return s.store.SetTheme(t) }

// SetLanguage sets the language: "it" or "en" (config.write_failed on error).
func (s *Service) SetLanguage(l i18n.Lang) (Snapshot, error) { return s.store.SetLanguage(l) }

// Logs returns the most recent log entries, newest first. level is the
// minimum level ("debug", "info", "warn", "error"; "" for all), limit the
// maximum count (0 for the default). logs.read_failed on error.
func (s *Service) Logs(level string, limit int) ([]logging.Entry, error) {
	return logging.Read(s.logsDir, logging.Filter{Level: level, Limit: limit})
}

// PreviewTokens renders a token template (file path, caption, sheet name) as
// it would be right now, in the current language. A malformed template
// returns its token.* error with the 1-based character position in "pos".
func (s *Service) PreviewTokens(template string) (string, error) {
	return tokens.Render(template, s.clock.Now(), s.store.Snapshot().Language)
}
