package app

import (
	"errors"
	"testing"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
)

type fakeInhibitor struct {
	on   bool
	fail bool
}

func (f *fakeInhibitor) Enable() error {
	if f.fail {
		return errors.New("refused")
	}
	f.on = true
	return nil
}
func (f *fakeInhibitor) Disable() error { f.on = false; return nil }

type fakeAutostart struct {
	on   bool
	fail bool
}

func (f *fakeAutostart) Enabled() (bool, error) { return f.on, nil }
func (f *fakeAutostart) Enable() error {
	if f.fail {
		return errors.New("denied")
	}
	f.on = true
	return nil
}
func (f *fakeAutostart) Disable() error { f.on = false; return nil }

func TestDefaults(t *testing.T) {
	s := NewStore(&fakeInhibitor{}, &fakeAutostart{}, true).Snapshot()
	if !s.RoutineActive || s.KeepAwake || !s.Autostart || s.Theme != ThemeSystem || s.Language != i18n.IT || s.WhatsApp != WADisconnected {
		t.Fatalf("defaults: %+v", s)
	}
}

func TestListenersGetEveryChangeOnce(t *testing.T) {
	st := NewStore(&fakeInhibitor{}, &fakeAutostart{}, false)
	var got []Snapshot
	st.Subscribe(func(s Snapshot) { got = append(got, s) })
	if _, err := st.SetRoutineActive(false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetRoutineActive(false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetLanguage(i18n.EN); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].RoutineActive || got[1].Language != i18n.EN {
		t.Fatalf("notifications: %+v", got)
	}
}

func TestKeepAwakeFailureKeepsState(t *testing.T) {
	inh := &fakeInhibitor{fail: true}
	st := NewStore(inh, &fakeAutostart{}, false)
	notified := false
	st.Subscribe(func(Snapshot) { notified = true })
	s, err := st.SetKeepAwake(true)
	if apperr.As(err).Code != apperr.KeepAwakeFailed {
		t.Fatalf("err %v", err)
	}
	if s.KeepAwake || notified {
		t.Fatal("state must not change on failure")
	}
	inh.fail = false
	if s, err = st.SetKeepAwake(true); err != nil || !s.KeepAwake || !inh.on {
		t.Fatalf("s=%+v err=%v", s, err)
	}
	if err := st.Shutdown(); err != nil || inh.on {
		t.Fatal("shutdown must release keep awake")
	}
}

func TestAutostartFailure(t *testing.T) {
	st := NewStore(&fakeInhibitor{}, &fakeAutostart{fail: true}, false)
	if _, err := st.SetAutostart(true); apperr.As(err).Code != apperr.AutostartFailed {
		t.Fatalf("err %v", err)
	}
	if st.Snapshot().Autostart {
		t.Fatal("state must not change on failure")
	}
}

func TestInvalidValues(t *testing.T) {
	st := NewStore(&fakeInhibitor{}, &fakeAutostart{}, false)
	if _, err := st.SetTheme("blue"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := st.SetLanguage("fr"); err == nil {
		t.Fatal("expected error")
	}
	if s, err := st.SetTheme(ThemeDark); err != nil || s.Theme != ThemeDark {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}
