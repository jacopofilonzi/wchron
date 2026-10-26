package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Rome everywhere (tests only)

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/provider/fake"
)

func TestPreviewTokens(t *testing.T) {
	st := newStore(&fakeInhibitor{}, &fakeAutostart{}, false)
	clk := clock.NewFake(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC))
	svc := NewService(st, provider.NewRegistry(), nil, nil, t.TempDir(), clk)

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
	svc := NewService(st, provider.NewRegistry(), nil, nil, t.TempDir(), clock.NewFake(time.Date(2026, 10, 24, 23, 0, 0, 0, loc)))
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

func TestChannelBindings(t *testing.T) {
	st := newStore(&fakeInhibitor{}, &fakeAutostart{}, false)
	fp := fake.New()
	fp.Unpaired = true
	reg := provider.NewRegistry(fp)
	var events []provider.PairingEvent
	mgr := provider.NewManager(provider.Options{Registry: reg, Dir: t.TempDir(), Store: st, OnChange: st.SetChannels,
		OnPairing: func(p provider.PairingEvent) { events = append(events, p) }})
	defer mgr.Close()
	mgr.Start()
	svc := NewService(st, reg, mgr, nil, t.TempDir(), clock.Real{})

	if ps := svc.Providers(); len(ps) != 1 || ps[0].Type != fake.Type || !ps[0].Capabilities.Pairing {
		t.Fatalf("providers: %+v", ps)
	}
	info, err := svc.CreateChannel(fake.Type, "Test")
	if err != nil {
		t.Fatal(err)
	}
	waitState := func(want provider.State) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			if l := st.Snapshot().ChannelList; len(l) == 1 && l[0].Status.State == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("channel never reached %s: %+v", want, st.Snapshot().ChannelList)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	waitState(provider.StateNotPaired)
	if st.Snapshot().Channels != ChannelsDisconnected {
		t.Fatalf("summary: %v", st.Snapshot().Channels)
	}
	if _, err := svc.StartPairing(info.ID); err != nil {
		t.Fatal(err)
	}
	waitState(provider.StateConnected)
	if st.Snapshot().Channels != ChannelsConnected {
		t.Fatalf("summary: %v", st.Snapshot().Channels)
	}
	chk, err := svc.ValidateTarget(t.Context(), info.ID, json.RawMessage(`{"to":"x"}`))
	if err != nil || chk.Verified != config.VerifiedYes {
		t.Fatalf("validate: %+v %v", chk, err)
	}
	if res, err := svc.DeleteChannel(info.ID); err != nil || !res.LoggedOut || len(svc.ListChannels()) != 0 ||
		st.Snapshot().Channels != ChannelsNone || len(st.Config().Channels) != 0 {
		t.Fatalf("delete: %+v %v", res, err)
	}
}

type fakeRuns struct {
	calls []string
	err   error
}

func (f *fakeRuns) RunNow(id, name string) error {
	f.calls = append(f.calls, id+" "+name)
	return f.err
}

func TestSendNow(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{{ID: "s", Name: "Sales"}}
	st := NewStore(Deps{Inhibitor: &fakeInhibitor{}, Autostart: &fakeAutostart{}, Config: cfg})
	runs := &fakeRuns{}
	svc := NewService(st, provider.NewRegistry(), nil, runs, t.TempDir(), clock.Real{})
	if err := svc.SendNow("s"); err != nil || len(runs.calls) != 1 || runs.calls[0] != "s Sales" {
		t.Fatalf("send now: %v %v", err, runs.calls)
	}
	runs.err = apperr.New(apperr.ScheduleBusy, nil)
	if err := svc.SendNow("s"); apperr.As(err).Code != apperr.ScheduleBusy {
		t.Fatalf("busy: %v", err)
	}
	if err := svc.SendNow("x"); apperr.As(err).Code != apperr.ScheduleNotFound {
		t.Fatalf("missing: %v", err)
	}
}

func TestRunStateAndAck(t *testing.T) {
	st := newStore(&fakeInhibitor{}, &fakeAutostart{}, false)
	var snaps []Snapshot
	st.Subscribe(func(s Snapshot) { snaps = append(snaps, s) })
	st.RunStarted("a")
	st.RunStarted("b")
	if s := st.Snapshot(); !s.Sending || s.RunError {
		t.Fatalf("running: %+v", s)
	}
	st.RunFinished("a", "failed")
	if s := st.Snapshot(); !s.Sending || !s.RunError {
		t.Fatalf("a failed, b running: %+v", s)
	}
	st.RunFinished("b", "skipped")
	if s := st.Snapshot(); s.Sending || !s.RunError {
		t.Fatalf("all done: %+v", s)
	}
	// a succeeds again: the error goes.
	st.RunStarted("a")
	st.RunFinished("a", "ok")
	if s := st.Snapshot(); s.RunError {
		t.Fatal("error state must clear when the schedule succeeds again")
	}
	// Ack clears it too.
	st.RunStarted("a")
	st.RunFinished("a", "partial")
	if s := st.AckErrors(); s.RunError {
		t.Fatal("ack must clear the error state")
	}
	if len(snaps) == 0 || snaps[len(snaps)-1].RunError {
		t.Fatal("listeners must see the ack")
	}
}
