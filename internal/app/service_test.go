package app

import (
	"errors"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Rome everywhere (tests only)

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
)

func TestPreviewTokens(t *testing.T) {
	st := newStore(&fakeInhibitor{}, &fakeAutostart{}, false)
	clk := clock.NewFake(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC))
	svc := NewService(st, t.TempDir(), clk)

	got, err := svc.PreviewTokens("{date-1d:dddd DD MMMM}")
	if err != nil || got != "domenica 25 ottobre" {
		t.Fatalf("it: got %q, %v", got, err)
	}
	if _, err := st.SetLanguage(i18n.EN); err != nil {
		t.Fatal(err)
	}
	clk.Advance(24 * time.Hour)
	if got, err = svc.PreviewTokens("{date-1d:dddd DD MMMM}"); err != nil || got != "Monday 26 October" {
		t.Fatalf("en, next day: got %q, %v", got, err)
	}

	_, err = svc.PreviewTokens("è{date")
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != apperr.TokenUnclosed || e.Params["pos"] != "2" {
		t.Fatalf("got %v", err)
	}
}

func TestPreviewRuns(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatal(err)
	}
	st := newStore(&fakeInhibitor{}, &fakeAutostart{}, false)
	svc := NewService(st, t.TempDir(), clock.NewFake(time.Date(2026, 10, 24, 23, 0, 0, 0, loc)))
	svc.loc = loc
	c := config.Cadence{Mode: config.CadenceInterval, Interval: config.Interval{Hours: 1}, Days: []config.Weekday{"sun"}}
	got, err := svc.PreviewRuns(c, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Sunday 25 October 2026: the DST change; 02:00 runs once, at +02:00.
	want := []string{"2026-10-25T00:00:00+02:00", "2026-10-25T01:00:00+02:00", "2026-10-25T02:00:00+02:00",
		"2026-10-25T03:00:00+01:00", "2026-10-25T04:00:00+01:00"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v", got)
	}
	if got, _ := svc.PreviewRuns(c, 1000); len(got) != 50 {
		t.Fatalf("n is capped at 50, got %d", len(got))
	}
	c.Days = nil
	_, err = svc.PreviewRuns(c, 5)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != apperr.ScheduleNoDays {
		t.Fatalf("got %v", err)
	}
}
