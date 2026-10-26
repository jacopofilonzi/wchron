package app

// Run history for the Status page, read back from the structured log (no
// history file): the lines the pipeline and the scheduler write for every
// run and skip.
//
//   - "pipeline: run finished": schedule, name, manual, status, sent,
//     planned, error_code / error_params (the most severe problem, masked);
//   - "pipeline: run skipped, too late for the first send": schedule, name,
//     code, params (slot.late);
//   - "scheduler: slot skipped" and "scheduler: slots skipped while paused":
//     schedule, code, params (params.schedule is the name).
//
// Lines from versions that didn't log the name fall back to the schedule's
// current name, then to its id. The log rotates (5 files of about 5 MB), so
// the history goes back as far as the log does.

import (
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/state"
)

// Log messages the history is built from (written by pipeline and scheduler).
const (
	msgRunFinished   = "pipeline: run finished"
	msgRunLate       = "pipeline: run skipped, too late for the first send"
	msgSlotSkipped   = "scheduler: slot skipped"
	msgPausedSkipped = "scheduler: slots skipped while paused"
)

// RunRecord is one run (or skip) in the history.
type RunRecord struct {
	Time         time.Time `json:"time"`
	ScheduleID   string    `json:"schedule_id"`
	ScheduleName string    `json:"schedule_name"`
	Manual       bool      `json:"manual"`
	// Status is a state.Result* value.
	Status  string `json:"status"`
	Sent    int    `json:"sent"`
	Planned int    `json:"planned"`
	// Error is the most severe problem or the skip reason; null when none.
	Error *apperr.Ref `json:"error"`
}

// History bounds for RunHistory.
const (
	historyDefault = 50
	historyMax     = 500
)

// RunHistory returns the most recent runs and skips, newest first, from the
// log (limit 0 = 50, at most 500). logs.read_failed on error.
func (s *Service) RunHistory(limit int) ([]RunRecord, error) {
	if limit <= 0 {
		limit = historyDefault
	}
	limit = min(limit, historyMax)
	entries, err := logging.Read(s.logsDir, logging.Filter{Level: "info", Limit: logging.MaxLimit})
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, sc := range s.store.Config().Schedules {
		names[sc.ID] = sc.Name
	}
	out := []RunRecord{}
	for _, e := range entries {
		r, ok := runRecord(e)
		if !ok {
			continue
		}
		if r.ScheduleName == "" {
			r.ScheduleName = names[r.ScheduleID]
		}
		if r.ScheduleName == "" {
			r.ScheduleName = r.ScheduleID
		}
		out = append(out, r)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func runRecord(e logging.Entry) (RunRecord, bool) {
	r := RunRecord{Time: e.Time, ScheduleID: str(e.Attrs["schedule"])}
	switch e.Msg {
	case msgRunFinished:
		r.ScheduleName = str(e.Attrs["name"])
		r.Manual, _ = e.Attrs["manual"].(bool)
		r.Status = str(e.Attrs["status"])
		r.Sent, r.Planned = num(e.Attrs["sent"]), num(e.Attrs["planned"])
		if c := str(e.Attrs["error_code"]); c != "" {
			r.Error = &apperr.Ref{Code: apperr.Code(c), Params: params(e.Attrs["error_params"])}
		}
	case msgRunLate, msgSlotSkipped, msgPausedSkipped:
		r.Status = state.ResultSkipped
		p := params(e.Attrs["params"])
		r.ScheduleName = str(e.Attrs["name"])
		if r.ScheduleName == "" {
			r.ScheduleName = p["schedule"]
		}
		r.Error = &apperr.Ref{Code: apperr.Code(str(e.Attrs["code"])), Params: p}
	default:
		return RunRecord{}, false
	}
	return r, r.ScheduleID != "" && r.Status != ""
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// num reads a JSON number (float64 after decoding).
func num(v any) int {
	f, _ := v.(float64)
	return int(f)
}

func params(v any) map[string]string {
	out := map[string]string{}
	m, _ := v.(map[string]any)
	for k, x := range m {
		if s, ok := x.(string); ok {
			out[k] = s
		}
	}
	return out
}
