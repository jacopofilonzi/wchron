package scheduler

import (
	"errors"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Rome on machines without Go's zoneinfo (tests only, never the app)

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
)

func rome(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

var allDays = []config.Weekday{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

func every(h, m int, anchor string, window *config.Window, days ...config.Weekday) config.Cadence {
	if len(days) == 0 {
		days = allDays
	}
	return config.Cadence{Mode: config.CadenceInterval, Interval: config.Interval{Hours: h, Minutes: m},
		Anchor: anchor, Window: window, Days: days}
}

func fixed(times []string, days ...config.Weekday) config.Cadence {
	if len(days) == 0 {
		days = allDays
	}
	return config.Cadence{Mode: config.CadenceFixed, Times: times, Days: days}
}

func win(s, e string) *config.Window { return &config.Window{Start: s, End: e} }

// fmtSlot shows a slot as "MM-DD Mon HH:MM +hh" (local wall clock and offset).
func fmtSlot(t time.Time) string { return t.Format("01-02 Mon 15:04 -07") }

func TestNext(t *testing.T) {
	loc := rome(t)
	at := func(mo time.Month, d, h, m, s int) time.Time { return time.Date(2026, mo, d, h, m, s, 0, loc) }
	tests := []struct {
		name string
		c    config.Cadence
		from time.Time
		n    int
		want []string
	}{
		{"interval divides the day", every(6, 0, "", nil), at(5, 31, 23, 59, 0), 5,
			[]string{"06-01 Mon 00:00 +02", "06-01 Mon 06:00 +02", "06-01 Mon 12:00 +02", "06-01 Mon 18:00 +02", "06-02 Tue 00:00 +02"}},
		{"interval doesn't divide the day: restarts at the anchor", every(7, 0, "", nil), at(6, 1, 0, 0, 0), 4,
			[]string{"06-01 Mon 07:00 +02", "06-01 Mon 14:00 +02", "06-01 Mon 21:00 +02", "06-02 Tue 00:00 +02"}},
		{"from is excluded", every(7, 0, "", nil), at(6, 1, 7, 0, 0), 1, []string{"06-01 Mon 14:00 +02"}},
		{"1 minute", every(0, 1, "", nil), at(6, 1, 10, 0, 30), 3,
			[]string{"06-01 Mon 10:01 +02", "06-01 Mon 10:02 +02", "06-01 Mon 10:03 +02"}},
		{"1 minute across midnight", every(0, 1, "", nil), at(6, 1, 23, 58, 0), 3,
			[]string{"06-01 Mon 23:59 +02", "06-02 Tue 00:00 +02", "06-02 Tue 00:01 +02"}},
		{"24h", every(24, 0, "09:30", nil), at(6, 1, 10, 0, 0), 2, []string{"06-02 Tue 09:30 +02", "06-03 Wed 09:30 +02"}},
		{"anchor not at midnight: nothing before it", every(4, 0, "08:15", nil), at(6, 1, 0, 0, 0), 5,
			[]string{"06-01 Mon 08:15 +02", "06-01 Mon 12:15 +02", "06-01 Mon 16:15 +02", "06-01 Mon 20:15 +02", "06-02 Tue 08:15 +02"}},
		{"window end is inclusive", every(1, 0, "", win("08:00", "20:00")), at(6, 1, 19, 30, 0), 3,
			[]string{"06-01 Mon 20:00 +02", "06-02 Tue 08:00 +02", "06-02 Tue 09:00 +02"}},
		{"window start is inclusive", every(1, 30, "", win("09:00", "12:00")), at(6, 1, 0, 0, 0), 3,
			[]string{"06-01 Mon 09:00 +02", "06-01 Mon 10:30 +02", "06-01 Mon 12:00 +02"}},
		{"narrow window", every(1, 0, "", win("08:30", "09:30")), at(6, 1, 0, 0, 0), 2,
			[]string{"06-01 Mon 09:00 +02", "06-02 Tue 09:00 +02"}},
		{"overnight window: after midnight belongs to the opening day", every(1, 0, "", win("22:00", "02:00"), "mon"),
			at(5, 31, 21, 0, 0), 6, // Sunday is inactive: Monday 00:00–02:00 doesn't fire
			[]string{"06-01 Mon 22:00 +02", "06-01 Mon 23:00 +02", "06-02 Tue 00:00 +02", "06-02 Tue 01:00 +02",
				"06-02 Tue 02:00 +02", "06-08 Mon 22:00 +02"}},
		{"overnight window: next day's anchor + k·interval", every(1, 30, "", win("22:00", "02:00"), "mon"),
			at(6, 1, 0, 0, 0), 4,
			[]string{"06-01 Mon 22:30 +02", "06-02 Tue 00:00 +02", "06-02 Tue 01:30 +02", "06-08 Mon 22:30 +02"}},
		{"overnight window on the last active day of the week", every(2, 0, "", win("23:00", "01:00"), "sun"),
			at(6, 6, 12, 0, 0), 2, []string{"06-08 Mon 00:00 +02", "06-15 Mon 00:00 +02"}},
		{"fixed times, sorted, across the week", fixed([]string{"14:00", "09:00"}, "sat", "sun"), at(6, 5, 12, 0, 0), 5,
			[]string{"06-06 Sat 09:00 +02", "06-06 Sat 14:00 +02", "06-07 Sun 09:00 +02", "06-07 Sun 14:00 +02", "06-13 Sat 09:00 +02"}},
		{"fixed ignores the window", config.Cadence{Mode: config.CadenceFixed, Times: []string{"07:00"},
			Window: win("08:00", "09:00"), Days: allDays}, at(6, 1, 0, 0, 0), 1, []string{"06-01 Mon 07:00 +02"}},
		{"days only", every(24, 0, "12:00", nil, "wed"), at(6, 1, 0, 0, 0), 3,
			[]string{"06-03 Wed 12:00 +02", "06-10 Wed 12:00 +02", "06-17 Wed 12:00 +02"}},
		{"DST spring: the missing hour is skipped", every(0, 30, "", win("01:00", "04:00")), at(3, 29, 0, 0, 0), 5,
			[]string{"03-29 Sun 01:00 +01", "03-29 Sun 01:30 +01", "03-29 Sun 03:00 +02", "03-29 Sun 03:30 +02", "03-29 Sun 04:00 +02"}},
		{"DST autumn: the repeated hour fires once", every(0, 30, "", win("01:00", "04:00")), at(10, 25, 0, 0, 0), 7,
			[]string{"10-25 Sun 01:00 +02", "10-25 Sun 01:30 +02", "10-25 Sun 02:00 +02", "10-25 Sun 02:30 +02",
				"10-25 Sun 03:00 +01", "10-25 Sun 03:30 +01", "10-25 Sun 04:00 +01"}},
		{"DST spring, overnight window", every(1, 0, "", win("22:00", "04:00"), "sat"), at(3, 28, 12, 0, 0), 6,
			[]string{"03-28 Sat 22:00 +01", "03-28 Sat 23:00 +01", "03-29 Sun 00:00 +01", "03-29 Sun 01:00 +01",
				"03-29 Sun 03:00 +02", "03-29 Sun 04:00 +02"}},
		{"DST autumn, overnight window", every(1, 0, "", win("22:00", "04:00"), "sat"), at(10, 24, 12, 0, 0), 8,
			[]string{"10-24 Sat 22:00 +02", "10-24 Sat 23:00 +02", "10-25 Sun 00:00 +02", "10-25 Sun 01:00 +02",
				"10-25 Sun 02:00 +02", "10-25 Sun 03:00 +01", "10-25 Sun 04:00 +01", "10-31 Sat 22:00 +01"}},
		{"from inside the repeated hour, second pass", every(1, 0, "", nil), time.Date(2026, 10, 25, 1, 10, 0, 0, time.UTC), 1,
			[]string{"10-25 Sun 03:00 +01"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Next(tc.c, tc.from, tc.n, loc)
			if err != nil {
				t.Fatal(err)
			}
			var s []string
			for _, g := range got {
				s = append(s, fmtSlot(g))
			}
			if strings.Join(s, ", ") != strings.Join(tc.want, ", ") {
				t.Errorf("got  %v\nwant %v", s, tc.want)
			}
		})
	}
}

func TestDayEventsDST(t *testing.T) {
	loc := rome(t)
	p, err := compile(every(0, 30, "", win("01:30", "03:30")))
	if err != nil {
		t.Fatal(err)
	}
	show := func(evs []event) string {
		var s []string
		for _, e := range evs {
			s = append(s, []string{"fire", "repeat", "gap"}[e.kind]+" "+e.wall.Format("15:04")+" @"+e.at.UTC().Format("15:04Z"))
		}
		return strings.Join(s, ", ")
	}
	// Spring: 02:00 and 02:30 don't exist; the gap events sit where
	// time.Date puts them, after the change.
	if got, want := show(p.dayEvents(2026, 3, 29, loc)),
		"fire 01:30 @00:30Z, gap 02:00 @01:00Z, fire 03:00 @01:00Z, gap 02:30 @01:30Z, fire 03:30 @01:30Z"; got != want {
		t.Errorf("spring:\n got  %s\n want %s", got, want)
	}
	// Autumn: 02:00 and 02:30 happen twice; the second time is a repeat.
	if got, want := show(p.dayEvents(2026, 10, 25, loc)),
		"fire 01:30 @23:30Z, fire 02:00 @00:00Z, fire 02:30 @00:30Z, repeat 02:00 @01:00Z, repeat 02:30 @01:30Z, fire 03:00 @02:00Z, fire 03:30 @02:30Z"; got != want {
		t.Errorf("autumn:\n got  %s\n want %s", got, want)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		c     config.Cadence
		code  apperr.Code
		value string
	}{
		{"1 minute", every(0, 1, "", nil), "", ""},
		{"24h", every(24, 0, "", nil), "", ""},
		{"zero interval", every(0, 0, "", nil), apperr.CadenceIntervalInvalid, ""},
		{"over 24h", every(24, 1, "", nil), apperr.CadenceIntervalInvalid, ""},
		{"60 minutes", every(0, 60, "", nil), apperr.CadenceIntervalInvalid, ""},
		{"negative hours", every(-1, 30, "", nil), apperr.CadenceIntervalInvalid, ""},
		{"anchor", every(1, 0, "08:30", nil), "", ""},
		{"anchor without padding", every(1, 0, "8:30", nil), apperr.CadenceAnchorInvalid, "8:30"},
		{"anchor 24:00", every(1, 0, "24:00", nil), apperr.CadenceAnchorInvalid, "24:00"},
		{"anchor minutes", every(1, 0, "10:60", nil), apperr.CadenceAnchorInvalid, "10:60"},
		{"window", every(1, 0, "", win("08:00", "20:00")), "", ""},
		{"overnight window", every(1, 0, "", win("22:00", "02:00")), "", ""},
		{"window start == end", every(1, 0, "", win("08:00", "08:00")), apperr.CadenceWindowInvalid, ""},
		{"window bad time", every(1, 0, "", win("08:00", "25:00")), apperr.CadenceWindowInvalid, ""},
		{"window missing end", every(1, 0, "", win("08:00", "")), apperr.CadenceWindowInvalid, ""},
		{"window with no slot", every(2, 0, "", win("09:00", "09:59")), apperr.CadenceWindowEmpty, ""},
		{"overnight window with no slot", every(24, 0, "12:00", win("22:00", "02:00")), apperr.CadenceWindowEmpty, ""},
		{"fixed", fixed([]string{"09:00", "18:30"}), "", ""},
		{"fixed without times", fixed(nil), apperr.CadenceNoTimes, ""},
		{"fixed bad time", fixed([]string{"09:00", "9:00"}), apperr.CadenceTimeInvalid, "9:00"},
		{"fixed duplicate", fixed([]string{"09:00", "12:00", "09:00"}), apperr.CadenceTimeDuplicate, "09:00"},
		{"fixed with an invalid window (not used)", config.Cadence{Mode: config.CadenceFixed, Times: []string{"09:00"},
			Window: win("x", "x"), Days: allDays}, "", ""},
		{"no days", config.Cadence{Mode: config.CadenceFixed, Times: []string{"09:00"}, Days: []config.Weekday{}},
			apperr.ScheduleNoDays, ""},
		{"unknown mode (structural check missed it)", config.Cadence{Mode: "cron", Days: allDays}, apperr.Internal, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.c)
			if tc.code == "" {
				if err != nil {
					t.Fatalf("unexpected %v", err)
				}
				return
			}
			var e *apperr.Error
			if !errors.As(err, &e) || e.Code != tc.code || e.Params["value"] != tc.value {
				t.Fatalf("got %v (params %v), want %s value %q", err, e, tc.code, tc.value)
			}
		})
	}
	if _, err := Next(fixed(nil), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 5, time.UTC); err == nil {
		t.Fatal("Next must return the validation error")
	}
}
