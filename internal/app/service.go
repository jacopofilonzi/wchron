package app

import (
	"time"

	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/scheduler"
	"github.com/jacopofilonzi/wchron/internal/tokens"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventState carries the full Snapshot after every change (backend → frontend),
// including the next scheduled run (Snapshot.NextRun): there is no separate
// schedule event.
const EventState = "app:state"

// The Store is the scheduler's view of the app state.
var _ scheduler.Store = (*Store)(nil)

func init() {
	application.RegisterEvent[Snapshot](EventState)
}

// Service is the Wails service bound to the frontend. Each method returns the
// new state, so the caller doesn't have to wait for EventState.
type Service struct {
	store   *Store
	logsDir string
	clock   clock.Clock
	loc     *time.Location // time.Local; tests set Europe/Rome
}

// NewService wraps the store for the bindings; logsDir is where Logs reads,
// clk the source of "now" for the previews (clock.Real in the app).
func NewService(st *Store, logsDir string, clk clock.Clock) *Service {
	return &Service{store: st, logsDir: logsDir, clock: clk, loc: time.Local}
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

// Preview bounds for PreviewRuns.
const (
	previewDefault = 5
	previewMax     = 50
)

// PreviewRuns returns the next n run times of a cadence from now (RFC 3339,
// local offset), for the editor preview; n 0 means 5, at most 50. An invalid
// cadence returns its cadence.* or schedule.no_days error.
func (s *Service) PreviewRuns(c config.Cadence, n int) ([]string, error) {
	if n <= 0 {
		n = previewDefault
	}
	n = min(n, previewMax)
	runs, err := scheduler.Next(c, s.clock.Now(), n, s.loc)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.Format(time.RFC3339)
	}
	return out, nil
}
