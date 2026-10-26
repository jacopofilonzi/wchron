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
//	             "last_error": {"code", "params"} | null, "last_run_at": RFC 3339,
//	             "problems": [{"code", "params"}],
//	             "notified_signature": "…", "notified_at": RFC 3339}
//	  }
//	}
//
// last_slot is the instant of the last slot that started a run, with its UTC
// offset, so the two occurrences of a wall-clock slot in the repeated autumn
// hour differ: the scheduler uses it to drop the second one (slot.dst_repeat)
// and never to run the same slot twice. last_run_at is when that run started.
// last_result is one of the Result* values; last_error is the code of a
// failure or of a skip reason (slot.missed, slot.paused…), null when the last
// run went fine. Skips don't change last_slot or last_run_at. problems lists
// every problem of the last run per file and recipient (warnings and
// notify.failed included; a skip lists only its reason). A manual run
// ("send now") sets last_run_at, never last_slot.
//
// notified_signature and notified_at are the error notifications' memory
// (package notify): the signature of the problem set last reported to the
// desktop and the error recipients, and when; empty once a "resolved"
// notice went out. They were added to version 1 in place (not released yet).
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
	LastSlot   time.Time  `json:"last_slot,omitzero"`
	LastResult string     `json:"last_result"`
	LastError  *ErrorRef  `json:"last_error"`
	LastRunAt  time.Time  `json:"last_run_at,omitzero"`
	Problems   []ErrorRef `json:"problems"`
	// NotifiedSignature and NotifiedAt: see the package doc (notify).
	NotifiedSignature string    `json:"notified_signature,omitempty"`
	NotifiedAt        time.Time `json:"notified_at,omitzero"`
}

// The values of ScheduleState.LastResult.
const (
	// ResultOK: every planned send went out (warnings may be listed in
	// Problems).
	ResultOK = "ok"
	// ResultPartial: some sends went out, some failed or were skipped.
	ResultPartial = "partial"
	// ResultDryRun: the run was only logged by the stub runner of versions
	// before 0.6.0; nothing was sent. Still readable in old files.
	ResultDryRun = "dry_run"
	// ResultSkipped: the slot was not run; LastError holds the reason.
	ResultSkipped = "skipped"
	// ResultFailed: nothing was sent; LastError holds the most severe problem.
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
		}
		if sc.Problems == nil {
			sc.Problems = []ErrorRef{}
		}
		for i := range sc.Problems {
			if sc.Problems[i].Params == nil {
				sc.Problems[i].Params = map[string]string{}
			}
		}
		s.Schedules[id] = sc
	}
}
