package scheduler

import (
	"slices"
	"time"

	"github.com/jacopofilonzi/wchron/internal/config"
)

// kind says what the scheduler does with an event.
type kind int

const (
	// fire: the first (usually only) occurrence of a slot: it runs.
	fire kind = iota
	// repeat: the second occurrence of a slot in the repeated autumn hour
	// (slot.dst_repeat).
	repeat
	// gap: a slot whose wall-clock time doesn't exist (spring DST change,
	// slot.dst_gap); its instant is where time.Date normalizes it, just
	// after the gap, so the skip is reported at about the right moment.
	gap
)

// event is one moment the scheduler must act on.
type event struct {
	// wall is the slot's wall-clock date and time (seconds zero), as a
	// time.Date in UTC: the identity of the slot, the same for both
	// occurrences of a repeated slot.
	wall time.Time
	// at is the instant, in the schedule's location.
	at   time.Time
	kind kind
	// jitter is the start delay of a fire event, drawn once when it becomes
	// the schedule's next event.
	jitter    time.Duration
	jitterSet bool
}

// due is when the scheduler acts on e: the slot plus the jitter for a run.
func (e *event) due() time.Time { return e.at.Add(e.jitter) }

// slotText is how a slot is shown in params and logs: local wall clock.
func slotText(wall time.Time) string { return wall.Format("2006-01-02 15:04") }

// dayEvents returns the events of the slots on calendar day (y, m, d) in loc,
// sorted by instant.
func (p *plan) dayEvents(y int, mo time.Month, d int, loc *time.Location) []event {
	wd := time.Date(y, mo, d, 12, 0, 0, 0, time.UTC).Weekday()
	var out []event
	for _, m := range p.minutes {
		if !p.inWindow(m) || !p.active(wd, m) {
			continue
		}
		wall := time.Date(y, mo, d, m/60, m%60, 0, 0, time.UTC)
		occ := occurrences(wall, loc)
		switch len(occ) {
		case 0:
			out = append(out, event{wall: wall, at: time.Date(y, mo, d, m/60, m%60, 0, 0, loc), kind: gap})
		default:
			out = append(out, event{wall: wall, at: occ[0], kind: fire})
			for _, t := range occ[1:] {
				out = append(out, event{wall: wall, at: t, kind: repeat})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b event) int { return a.at.Compare(b.at) })
	return out
}

// occurrences returns every instant whose wall clock in loc is wall (given in
// UTC), earliest first: none in a spring gap, two in the repeated autumn hour.
// time.Date alone can't tell: it silently picks one of them (the second, for
// the autumn change) or shifts a nonexistent time. The candidate offsets are
// the zone's offsets around that day.
func occurrences(wall time.Time, loc *time.Location) []time.Time {
	base := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), 0, 0, loc)
	var offsets []int
	for _, t := range []time.Time{base.Add(-24 * time.Hour), base, base.Add(24 * time.Hour)} {
		_, off := t.Zone()
		if !slices.Contains(offsets, off) {
			offsets = append(offsets, off)
		}
	}
	var out []time.Time
	for _, off := range offsets {
		t := time.Unix(wall.Unix()-int64(off), 0).In(loc)
		if sameWall(t, wall) && !slices.ContainsFunc(out, t.Equal) {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, time.Time.Compare)
	return out
}

// sameWall reports whether t (in its own location) reads as wall (UTC) to the minute.
func sameWall(t, wall time.Time) bool {
	y, mo, d := t.Date()
	return y == wall.Year() && mo == wall.Month() && d == wall.Day() && t.Hour() == wall.Hour() && t.Minute() == wall.Minute()
}

// maxScanDays bounds the search for the next event: a valid plan has a slot
// in every week, so this is only a guard against a bug looping forever.
const maxScanDays = 400

// cursor walks the events of a plan after a given instant, generating one
// calendar day at a time.
type cursor struct {
	p     *plan
	loc   *time.Location
	after time.Time // events at or before this are dropped
	queue []event
	day   time.Time // next calendar day to generate (midnight, UTC)
	empty int       // days generated in a row with no event, for the guard
}

func newCursor(p *plan, loc *time.Location, after time.Time) *cursor {
	y, m, d := after.In(loc).Date()
	// Start one day early: a slot of the previous calendar day can't fall
	// after `after`, but generating it costs nothing and keeps the edge of
	// unusual zones (a DST change at midnight) simple.
	return &cursor{p: p, loc: loc, after: after, day: time.Date(y, m, d-1, 0, 0, 0, 0, time.UTC)}
}

// fill generates days until the queue holds index i and the day after the
// event at i, so that no event of a later day can sort before it.
func (c *cursor) fill(i int) bool {
	for {
		if len(c.queue) > i {
			last := c.queue[i].wall
			lastDay := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC)
			if c.day.After(lastDay.AddDate(0, 0, 1)) {
				return true
			}
		} else if c.empty > maxScanDays {
			return false
		}
		evs := c.p.dayEvents(c.day.Year(), c.day.Month(), c.day.Day(), c.loc)
		n := 0
		for _, e := range evs {
			if e.at.After(c.after) {
				c.queue = append(c.queue, e)
				n++
			}
		}
		if n == 0 {
			c.empty++
		} else {
			c.empty = 0
		}
		c.day = c.day.AddDate(0, 0, 1)
		slices.SortStableFunc(c.queue, func(a, b event) int { return a.at.Compare(b.at) })
	}
}

// peek returns the next event, or nil (only if the plan has none: a bug).
func (c *cursor) peek() *event {
	if !c.fill(0) {
		return nil
	}
	return &c.queue[0]
}

// pop drops the next event.
func (c *cursor) pop() {
	if len(c.queue) > 0 {
		c.after = c.queue[0].at
		c.queue = c.queue[1:]
	}
}

// nextFire returns the next event that would run, skipping gaps and repeats.
func (c *cursor) nextFire() *event {
	for i := 0; c.fill(i); i++ {
		if c.queue[i].kind == fire {
			return &c.queue[i]
		}
	}
	return nil
}

// Next returns the next n slots of a cadence strictly after from, in loc
// (time.Local in the app): the instants that would run, without jitter.
// Spring-gap slots are left out and a repeated autumn slot appears once.
// An invalid cadence returns its validation error.
func Next(c config.Cadence, from time.Time, n int, loc *time.Location) ([]time.Time, error) {
	p, err := compile(c)
	if err != nil {
		return nil, err
	}
	cur := newCursor(&p, loc, from)
	out := make([]time.Time, 0, n)
	for len(out) < n {
		e := cur.peek()
		if e == nil {
			break
		}
		if e.kind == fire {
			out = append(out, e.at)
		}
		cur.pop()
	}
	return out, nil
}
