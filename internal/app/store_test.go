package app

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/state"
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

func newStore(inh *fakeInhibitor, auto *fakeAutostart, autostartOn bool) *Store {
	return NewStore(Deps{Inhibitor: inh, Autostart: auto, AutostartOn: autostartOn, Config: config.Default(), State: state.Default()})
}

func TestDefaults(t *testing.T) {
	s := newStore(&fakeInhibitor{}, &fakeAutostart{}, true).Snapshot()
	if !s.RoutineActive || s.KeepAwake || s.KeepAwakeDefault || !s.Autostart || s.Theme != config.ThemeSystem ||
		s.Language != i18n.IT || s.Channels != ChannelsNone {
		t.Fatalf("defaults: %+v", s)
	}
}

func TestListenersGetEveryChangeOnce(t *testing.T) {
	st := newStore(&fakeInhibitor{}, &fakeAutostart{}, false)
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
	st := newStore(inh, &fakeAutostart{}, false)
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
	st := newStore(&fakeInhibitor{}, &fakeAutostart{fail: true}, false)
	if _, err := st.SetAutostart(true); apperr.As(err).Code != apperr.AutostartFailed {
		t.Fatalf("err %v", err)
	}
	if st.Snapshot().Autostart {
		t.Fatal("state must not change on failure")
	}
}

func TestInvalidValues(t *testing.T) {
	st := newStore(&fakeInhibitor{}, &fakeAutostart{}, false)
	if _, err := st.SetTheme("blue"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := st.SetLanguage("fr"); err == nil {
		t.Fatal("expected error")
	}
	if s, err := st.SetTheme(config.ThemeDark); err != nil || s.Theme != config.ThemeDark {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}

// The Store starts from the loaded files and writes every change back.
func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	cfgPath, statePath := filepath.Join(dir, "config.json"), filepath.Join(dir, "state.json")
	cfg := config.Default()
	cfg.Settings.Theme, cfg.Settings.Language, cfg.Settings.KeepAwakeDefault = config.ThemeLight, i18n.EN, true
	stt := state.Default()
	stt.RoutineActive = false
	inh := &fakeInhibitor{}
	st := NewStore(Deps{
		Inhibitor: inh, Autostart: &fakeAutostart{}, Config: cfg, State: stt,
		SaveConfig: func(c config.Config) error { return config.Save(cfgPath, c) },
		SaveState:  func(s state.State) error { return state.Save(statePath, s) },
	})
	s := st.Snapshot()
	if s.Theme != config.ThemeLight || s.Language != i18n.EN || s.RoutineActive || !s.KeepAwakeDefault || s.KeepAwake {
		t.Fatalf("initial: %+v", s)
	}
	if err := st.Start(); err != nil || !st.Snapshot().KeepAwake || !inh.on {
		t.Fatalf("start must apply the keep-awake default: %v", err)
	}
	if _, err := st.SetTheme(config.ThemeDark); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetLanguage(i18n.IT); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetKeepAwakeDefault(false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetRoutineActive(true); err != nil {
		t.Fatal(err)
	}
	l, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if set := l.Config.Settings; set.Theme != config.ThemeDark || set.Language != i18n.IT || set.KeepAwakeDefault {
		t.Fatalf("saved settings: %+v", set)
	}
	sl, err := state.Load(statePath)
	if err != nil || !sl.State.RoutineActive {
		t.Fatalf("saved state: %+v %v", sl, err)
	}
}

func TestConfigSaveFailureRollsBack(t *testing.T) {
	failCfg := func(config.Config) error { return errors.New("disk full") }
	failState := func(state.State) error { return apperr.New(apperr.StateWriteFailed, errors.New("disk full")) }
	st := NewStore(Deps{
		Inhibitor: &fakeInhibitor{}, Autostart: &fakeAutostart{}, Config: config.Default(), State: state.Default(),
		SaveConfig: failCfg, SaveState: failState,
	})
	notified := 0
	st.Subscribe(func(Snapshot) { notified++ })
	for name, set := range map[string]func() (Snapshot, error){
		"theme":     func() (Snapshot, error) { return st.SetTheme(config.ThemeDark) },
		"language":  func() (Snapshot, error) { return st.SetLanguage(i18n.EN) },
		"keepawake": func() (Snapshot, error) { return st.SetKeepAwakeDefault(true) },
	} {
		s, err := set()
		if apperr.As(err).Code != apperr.ConfigWriteFailed {
			t.Fatalf("%s: err %v", name, err)
		}
		if s.Theme != config.ThemeSystem || s.Language != i18n.IT || s.KeepAwakeDefault {
			t.Fatalf("%s: not rolled back: %+v", name, s)
		}
	}
	if notified != 0 {
		t.Fatal("a refused change must not notify")
	}
	// state.json failures only warn: the pause still applies.
	if s, err := st.SetRoutineActive(false); err != nil || s.RoutineActive {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}

func TestScheduleStateAndNextRun(t *testing.T) {
	var saved []state.State
	st := NewStore(Deps{Inhibitor: &fakeInhibitor{}, Autostart: &fakeAutostart{}, Config: config.Default(), State: state.Default(),
		SaveState: func(s state.State) error { saved = append(saved, s); return nil }})
	slot := time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC)
	st.UpdateScheduleState("a", func(s *state.ScheduleState) { s.LastSlot = slot })
	st.UpdateScheduleState("a", func(s *state.ScheduleState) { s.LastResult = state.ResultDryRun })
	got := st.ScheduleState("a")
	if !got.LastSlot.Equal(slot) || got.LastResult != state.ResultDryRun || len(saved) != 2 ||
		saved[1].Schedules["a"].LastResult != state.ResultDryRun {
		t.Fatalf("state %+v, saved %d", got, len(saved))
	}

	var snaps []Snapshot
	st.Subscribe(func(s Snapshot) { snaps = append(snaps, s) })
	n := &NextRun{ScheduleID: "a", ScheduleName: "Report", At: slot}
	st.SetNextRun(n)
	st.SetNextRun(&NextRun{ScheduleID: "a", ScheduleName: "Report", At: slot.In(time.Local)}) // same instant
	st.SetNextRun(nil)
	if len(snaps) != 2 || snaps[0].NextRun == nil || snaps[0].NextRun.ScheduleName != "Report" || snaps[1].NextRun != nil {
		t.Fatalf("snapshots %+v", snaps)
	}
	if !st.RoutineActive() {
		t.Fatal("routine active by default")
	}
}

func TestChannelsSummary(t *testing.T) {
	cfg := config.Default()
	cfg.Channels = []config.Channel{{ID: "a", Provider: "fake", Name: "A", Enabled: true}}
	st := NewStore(Deps{Inhibitor: &fakeInhibitor{}, Autostart: &fakeAutostart{}, Config: cfg, State: state.Default()})
	if s := st.Snapshot(); s.Channels != ChannelsDisconnected || s.ChannelList == nil {
		t.Fatalf("before the channels open: %+v", s)
	}
	on := func(id string, state provider.State, enabled bool) provider.ChannelInfo {
		return provider.ChannelInfo{ID: id, Enabled: enabled, Status: provider.Status{State: state}}
	}
	cases := []struct {
		list []provider.ChannelInfo
		want ChannelsState
	}{
		{nil, ChannelsNone},
		{[]provider.ChannelInfo{on("a", provider.StateConnected, true)}, ChannelsConnected},
		{[]provider.ChannelInfo{on("a", provider.StateConnected, true), on("b", provider.StateNotPaired, true)}, ChannelsDisconnected},
		{[]provider.ChannelInfo{on("a", provider.StateConnected, true), on("b", provider.StateError, false)}, ChannelsConnected},
		{[]provider.ChannelInfo{on("b", provider.StateError, false)}, ChannelsNone},
	}
	for i, c := range cases {
		st.SetChannels(c.list)
		if s := st.Snapshot(); s.Channels != c.want || s.ChannelList == nil || len(s.ChannelList) != len(c.list) {
			t.Errorf("case %d: got %v (%d)", i, s.Channels, len(s.ChannelList))
		}
	}
}

func TestUpdateConfigIsAtomic(t *testing.T) {
	var saved []config.Config
	fail := false
	st := NewStore(Deps{Inhibitor: &fakeInhibitor{}, Autostart: &fakeAutostart{}, Config: config.Default(), State: state.Default(),
		SaveConfig: func(c config.Config) error {
			if fail {
				return errors.New("disk full")
			}
			saved = append(saved, c)
			return nil
		}})
	add := func(c *config.Config) error {
		c.Channels = append(c.Channels, config.Channel{ID: "a", Provider: "fake", Name: "A"})
		return nil
	}
	if err := st.UpdateConfig(add); err != nil || len(st.Config().Channels) != 1 || len(saved) != 1 {
		t.Fatalf("update: %v", err)
	}
	fail = true
	err := st.UpdateConfig(func(c *config.Config) error { c.Channels[0].Name = "B"; return nil })
	if apperr.As(err).Code != apperr.ConfigWriteFailed || st.Config().Channels[0].Name != "A" {
		t.Fatalf("a failed save must change nothing: %v %+v", err, st.Config().Channels)
	}
	fail = false
	if err := st.UpdateConfig(func(*config.Config) error { return apperr.New(apperr.ChannelNotFound, nil) }); apperr.As(err).Code != apperr.ChannelNotFound {
		t.Fatalf("change error: %v", err)
	}
	// Config is a deep copy.
	c := st.Config()
	c.Channels[0].Name = "mutated"
	if st.Config().Channels[0].Name != "A" {
		t.Fatal("Config must return a copy")
	}
}
