package tray

import (
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/app"
	"github.com/jacopofilonzi/wchron/internal/i18n"
)

func TestNextLine(t *testing.T) {
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC) // Monday
	next := func(d int) *app.NextRun {
		return &app.NextRun{ScheduleName: "Report vendite", At: time.Date(2026, 10, d, 14, 0, 0, 0, time.UTC)}
	}
	tests := []struct {
		s    app.Snapshot
		want string
	}{
		{app.Snapshot{RoutineActive: true, Language: i18n.EN, NextRun: next(26)}, "Next: Report vendite 14:00"},
		{app.Snapshot{RoutineActive: true, Language: i18n.IT, NextRun: next(26)}, "Prossimo: Report vendite 14:00"},
		{app.Snapshot{RoutineActive: true, Language: i18n.IT, NextRun: next(27)}, "Prossimo: Report vendite mar 14:00"},
		{app.Snapshot{RoutineActive: true, Language: i18n.EN, NextRun: next(27)}, "Next: Report vendite Tue 14:00"},
		{app.Snapshot{RoutineActive: true, Language: i18n.EN}, "No scheduled send"},
		{app.Snapshot{RoutineActive: false, Language: i18n.IT, NextRun: next(26)}, "Routine in pausa: nessun invio"},
	}
	for _, tc := range tests {
		if got := nextLine(tc.s, now); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}
