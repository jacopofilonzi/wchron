// Package state is state.json: what the app remembers between runs, as
// opposed to what the user configures (config.json). It is durable data with
// the same versioning rules as config.json (migrations.go, see package
// migrate), but losing it is not fatal: it can always be rebuilt.
//
// Format (current version: CurrentVersion), snake_case JSON:
//
//	{
//	  "schema_version": 1,
//	  "routine_active": true,            // the tray pause survives a restart
//	  "last_app_version": "0.2.0",       // a change triggers the startup backup
//	  "notified_update_version": "",     // the last update already notified
//	  "schedules": {                     // by schedule id
//	    "<id>": {"last_slot": RFC 3339, "last_result": "…",
//	             "last_error": {"code", "params"} | null, "last_run_at": RFC 3339}
//	  }
//	}
//
// last_slot is the instant of the last slot that started a run, with its UTC
// offset, so the two occurrences of a wall-clock slot in the repeated autumn
// hour differ: the scheduler uses it to drop the second one (slot.dst_repeat)
// and never to run the same slot twice. last_run_at is when that run started.
// last_result is one of the Result* values; last_error is the code of a
// failure or of a skip reason (slot.missed, slot.paused…), null when the last
// run went fine. Skips don't change last_slot or last_run_at.
//
// Loading: a missing file gives the defaults. A file that can't be read or
// parsed is not fatal: Load returns the defaults plus a state.read_failed
// warning (Loaded.Corrupt), and the caller moves the bad file aside with
// Quarantine (state.json.corrupt-<timestamp>) after the startup backup. A
// newer schema_version is fatal like for config.json (config.newer_version),
// so a newer app's file is never overwritten; a failed migration step is fatal
// too (config.migration_failed). Saving is atomic (temp file + fsync +
// rename); a failure is a state.write_failed warning for the caller to log.
package state

import (
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

// State is the whole state.json.
type State struct {
	SchemaVersion         int                      `json:"schema_version"`
	RoutineActive         bool                     `json:"routine_active"`
	LastAppVersion        string                   `json:"last_app_version"`
	NotifiedUpdateVersion string                   `json:"notified_update_version"`
	Schedules             map[string]ScheduleState `json:"schedules"`
}

// ScheduleState is the memory of one schedule's last run.
type ScheduleState struct {
	LastSlot   time.Time `json:"last_slot,omitzero"`
	LastResult string    `json:"last_result"`
	LastError  *ErrorRef `json:"last_error"`
	LastRunAt  time.Time `json:"last_run_at,omitzero"`
}

// The values of ScheduleState.LastResult.
const (
	// ResultOK: the run completed (pipeline, milestone 6).
	ResultOK = "ok"
	// ResultDryRun: the run was only logged (the stub runner used until the
	// pipeline lands); nothing was sent.
	ResultDryRun = "dry_run"
	// ResultSkipped: the slot was not run; LastError holds the reason.
	ResultSkipped = "skipped"
	// ResultFailed: the run failed; LastError holds the error.
	ResultFailed = "failed"
)

// ErrorRef is an apperr.Error as stored: code and params, no message.
type ErrorRef struct {
	Code   apperr.Code       `json:"code"`
	Params map[string]string `json:"params"`
}

// Default is the state of a fresh install: the routine is active.
func Default() State {
	s := State{SchemaVersion: CurrentVersion, RoutineActive: true}
	s.Normalize()
	return s
}

// Normalize turns nil maps into empty ones.
func (s *State) Normalize() {
	if s.Schedules == nil {
		s.Schedules = map[string]ScheduleState{}
	}
	for id, sc := range s.Schedules {
		if sc.LastError != nil && sc.LastError.Params == nil {
			sc.LastError.Params = map[string]string{}
			s.Schedules[id] = sc
		}
	}
}
