package scheduler

import (
	"fmt"
	"slices"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
)

const minutesPerDay = 24 * 60

// plan is a validated cadence in a form ready for slot generation.
type plan struct {
	// minutes are the sorted minutes of the day (0–1439) of the day pattern,
	// before the window: anchor + k·interval in interval mode, the times in
	// fixed mode.
	minutes []int
	// window is the interval-mode window in minutes of the day; nil means
	// all day. start > end crosses midnight.
	window *[2]int
	days   [7]bool // by time.Weekday
}

// Validate checks a cadence: the cadence.* codes and schedule.no_days. It is
// the semantic half of config validation (see the config package doc): the
// editor calls it on save and the scheduler ignores (with a warning) a
// schedule that fails it, so a bad hand-edited cadence never blocks startup.
func Validate(c config.Cadence) error {
	_, err := compile(c)
	return err
}

func compile(c config.Cadence) (plan, error) {
	var p plan
	switch c.Mode {
	case config.CadenceInterval:
		iv := c.Interval.Hours*60 + c.Interval.Minutes
		if c.Interval.Hours < 0 || c.Interval.Minutes < 0 || c.Interval.Minutes > 59 || iv < 1 || iv > minutesPerDay {
			return p, apperr.New(apperr.CadenceIntervalInvalid,
				fmt.Errorf("interval %dh%02dm out of range", c.Interval.Hours, c.Interval.Minutes))
		}
		anchor := 0
		if c.Anchor != "" {
			a, ok := parseHHMM(c.Anchor)
			if !ok {
				return p, apperr.New(apperr.CadenceAnchorInvalid, fmt.Errorf("anchor %q", c.Anchor)).With("value", c.Anchor)
			}
			anchor = a
		}
		for m := anchor; m < minutesPerDay; m += iv {
			p.minutes = append(p.minutes, m)
		}
		if c.Window != nil {
			s, okS := parseHHMM(c.Window.Start)
			e, okE := parseHHMM(c.Window.End)
			if !okS || !okE || s == e {
				return p, apperr.New(apperr.CadenceWindowInvalid,
					fmt.Errorf("window %q-%q", c.Window.Start, c.Window.End))
			}
			p.window = &[2]int{s, e}
			if !slices.ContainsFunc(p.minutes, p.inWindow) {
				return p, apperr.New(apperr.CadenceWindowEmpty,
					fmt.Errorf("no slot of the interval in window %q-%q", c.Window.Start, c.Window.End))
			}
		}
	case config.CadenceFixed:
		if len(c.Times) == 0 {
			return p, apperr.New(apperr.CadenceNoTimes, fmt.Errorf("fixed mode without times"))
		}
		for _, t := range c.Times {
			m, ok := parseHHMM(t)
			if !ok {
				return p, apperr.New(apperr.CadenceTimeInvalid, fmt.Errorf("time %q", t)).With("value", t)
			}
			if slices.Contains(p.minutes, m) {
				return p, apperr.New(apperr.CadenceTimeDuplicate, fmt.Errorf("time %q twice", t)).With("value", t)
			}
			p.minutes = append(p.minutes, m)
		}
		slices.Sort(p.minutes)
	default:
		// Structural validation (config.invalid) rejects unknown modes on
		// load; reaching this is a bug in the caller.
		return p, apperr.New(apperr.Internal, fmt.Errorf("unknown cadence mode %q", c.Mode))
	}
	for _, d := range c.Days {
		i := slices.Index(config.Weekdays, d)
		if i < 0 {
			return p, apperr.New(apperr.Internal, fmt.Errorf("unknown day %q", d))
		}
		// config.Weekdays starts on Monday, time.Weekday on Sunday.
		p.days[(i+1)%7] = true
	}
	if !slices.Contains(p.days[:], true) {
		return p, apperr.New(apperr.ScheduleNoDays, fmt.Errorf("no active day"))
	}
	return p, nil
}

// inWindow reports whether minute m of a day is inside the window (both ends
// included). Without a window every minute is.
func (p *plan) inWindow(m int) bool {
	if p.window == nil {
		return true
	}
	s, e := p.window[0], p.window[1]
	if s < e {
		return s <= m && m <= e
	}
	return m >= s || m <= e
}

// active reports whether a slot at minute m of calendar day d is enabled by
// the active days. A slot after midnight in an overnight window belongs to
// the day the window opened, so its previous calendar day is checked.
func (p *plan) active(d time.Weekday, m int) bool {
	if p.window != nil && p.window[0] > p.window[1] && m <= p.window[1] {
		d = (d + 6) % 7
	}
	return p.days[d]
}

func (p *plan) equal(o *plan) bool {
	if (p.window == nil) != (o.window == nil) || (p.window != nil && *p.window != *o.window) {
		return false
	}
	return p.days == o.days && slices.Equal(p.minutes, o.minutes)
}

// parseHHMM parses a strict "HH:MM" (two digits each, 00:00–23:59) into
// minutes of the day.
func parseHHMM(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
