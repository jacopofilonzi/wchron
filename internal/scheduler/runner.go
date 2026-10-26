package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/state"
)

// Job is one run the scheduler asks for.
type Job struct {
	ScheduleID   string
	ScheduleName string
	// Slot is the exact slot instant (without jitter).
	Slot time.Time
	// Deadline is Slot + Tolerance: the runner re-checks it right before its
	// first send and skips the run with slot.late after it.
	Deadline time.Time
}

// Result is what a run reports; it becomes the schedule's last_result and
// last_error in state.json.
type Result struct {
	// Status is one of the state.Result* values.
	Status string
	// Err is the failure or skip reason (nil when the run went fine).
	Err *apperr.Error
}

// Runner executes a run. It is called on its own goroutine; ctx is cancelled
// when the scheduler stops. The pipeline (milestone 6) implements it.
type Runner interface {
	Run(ctx context.Context, job Job) Result
}

// DryRunner is the production runner until the pipeline lands: it only logs
// what would run. It still applies the slot.late check, like the pipeline will.
type DryRunner struct {
	Clock clock.Clock
	Log   *slog.Logger
}

// Run logs the job and returns state.ResultDryRun (or slot.late).
func (r DryRunner) Run(_ context.Context, job Job) Result {
	now := r.Clock.Now()
	if now.After(job.Deadline) {
		return Result{Status: state.ResultSkipped, Err: apperr.New(apperr.SlotLate,
			fmt.Errorf("started %s after the slot", now.Sub(job.Slot).Round(time.Second))).
			With("schedule", job.ScheduleName).With("slot", slotText(wallOf(job.Slot)))}
	}
	r.Log.Info("scheduler: would run (no pipeline yet)", "schedule", job.ScheduleID, "name", job.ScheduleName,
		"slot", job.Slot.Format(time.RFC3339))
	return Result{Status: state.ResultDryRun}
}

// wallOf is t's wall clock in its own location, as a UTC time.Date.
func wallOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, time.UTC)
}
