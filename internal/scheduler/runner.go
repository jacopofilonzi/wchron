package scheduler

import (
	"context"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

// Job is one run the scheduler asks for.
type Job struct {
	ScheduleID   string
	ScheduleName string
	// Slot is the exact slot instant (without jitter); for a manual run
	// ("send now") it is the time of the click.
	Slot time.Time
	// Deadline is Slot + Tolerance: the runner re-checks it right before its
	// first send and skips the run with slot.late after it. Zero for a
	// manual run, which has no tolerance.
	Deadline time.Time
	// Manual is a "send now" from the GUI: no days, window or tolerance;
	// its problems go to the desktop only, never to the error recipients.
	Manual bool
}

// Result is what a run reports; it becomes the schedule's last_result,
// last_error and problems in state.json.
type Result struct {
	// Status is one of the state.Result* values.
	Status string
	// Err is the most severe problem (the failure or skip reason); nil when
	// the run went fine.
	Err *apperr.Error
	// Problems are every problem of the run, per file and recipient
	// (warnings included), in the order they happened.
	Problems []*apperr.Error
}

// Runner executes a run. It is called on its own goroutine; ctx is cancelled
// when the scheduler stops. The pipeline implements it.
type Runner interface {
	Run(ctx context.Context, job Job) Result
}

// SlotText is a slot (or any instant) as the texts show it: the local wall
// clock "YYYY-MM-DD HH:MM" in t's location.
func SlotText(t time.Time) string { return t.Format("2006-01-02 15:04") }
