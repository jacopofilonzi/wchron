package app

import (
	"errors"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
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
