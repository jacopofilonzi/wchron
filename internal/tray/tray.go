// Package tray is the system tray icon and menu.
//
// Menu: checkbox "Activate routine" (global pause/resume), checkbox "Keep
// awake", a disabled status line (channels state; the last send comes with the
// pipeline), "Open window", "Quit". Labels come from the catalog (`tray.*`) and
// are relabelled in place when the language changes.
//
// Left click opens and focuses the window, right click opens the menu.
//
// Icon (see Icon and stateFor): the logo (internal/logo) with a status dot at
// the bottom right: active green, paused grey, sending cyan, error red,
// channels disconnected (or none configured) purple (not amber: it would read
// as the logo's orange); plus a yellow dot at the top right when keep-awake is
// on. The icon is drawn at run time and replaced on every state change.
//
// Every toggle goes through app.Store, and the tray redraws from the Store's
// snapshots only, so the menu, the icon and the window never disagree. A
// checkbox flips itself when clicked (Wails behaviour): if the Store refuses
// the change, the checkbox is set back from the current snapshot.
//
// Wails v3 beta notes (verified on beta.29): menu callbacks run on their own
// goroutine; SetChecked/SetLabel/SetEnabled update the live Win32 menu in
// place; SetIcon/SetTooltip dispatch to the main thread themselves; the click
// handlers run on the main thread, so they must not block.
package tray

import (
	"log/slog"
	"sync"

	"github.com/jacopofilonzi/wchron/internal/app"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Tray owns the system tray and its menu items.
type Tray struct {
	wails                                  *application.App
	store                                  *app.Store
	log                                    *slog.Logger
	show                                   func()
	tray                                   *application.SystemTray
	mu                                     sync.Mutex // apply runs from store listeners and from fail
	routine, keepAwake, status, open, quit *application.MenuItem
}

// New builds the tray from the current state and keeps it in sync with the
// store. showWindow brings the main window to the front.
func New(wails *application.App, store *app.Store, log *slog.Logger, showWindow func()) *Tray {
	t := &Tray{wails: wails, store: store, log: log, show: showWindow}
	s := store.Snapshot()

	menu := wails.NewMenu()
	t.routine = menu.AddCheckbox("", s.RoutineActive).OnClick(func(ctx *application.Context) {
		if _, err := store.SetRoutineActive(ctx.IsChecked()); err != nil {
			t.fail("routine toggle", err)
		}
	})
	t.keepAwake = menu.AddCheckbox("", s.KeepAwake).OnClick(func(ctx *application.Context) {
		if _, err := store.SetKeepAwake(ctx.IsChecked()); err != nil {
			t.fail("keep awake toggle", err)
		}
	})
	menu.AddSeparator()
	t.status = menu.Add("").SetEnabled(false)
	t.open = menu.Add("").OnClick(func(*application.Context) { showWindow() })
	menu.AddSeparator()
	t.quit = menu.Add("").OnClick(func(*application.Context) { wails.Quit() })

	t.tray = wails.SystemTray.New()
	t.apply(s)
	t.tray.SetMenu(menu)
	t.tray.OnClick(showWindow)
	t.tray.OnRightClick(t.tray.OpenMenu)

	store.Subscribe(t.apply)
	return t
}

// fail logs a refused toggle and puts the checkboxes back to the real state.
// keepawake.failed is also meant for a toast: it comes with the Notifier.
func (t *Tray) fail(what string, err error) {
	t.log.Error("tray: "+what+" failed", "err", err)
	t.apply(t.store.Snapshot())
}

// apply redraws everything from a snapshot. Setting an unchanged label or
// check is harmless, so there is no diffing.
func (t *Tray) apply(s app.Snapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	l := s.Language
	t.routine.SetLabel(i18n.T(l, "tray.routine", nil)).SetChecked(s.RoutineActive)
	t.keepAwake.SetLabel(i18n.T(l, "tray.keepawake", nil)).SetChecked(s.KeepAwake)
	ch := i18n.T(l, "tray.channels."+string(s.Channels), nil)
	t.status.SetLabel(i18n.T(l, "tray.status.channels", map[string]string{"state": ch}))
	t.open.SetLabel(i18n.T(l, "tray.open", nil))
	t.quit.SetLabel(i18n.T(l, "tray.quit", nil))

	st := stateFor(s.RoutineActive, s.Channels == app.ChannelsConnected)
	tip := i18n.T(l, "tray.tooltip", map[string]string{"state": i18n.T(l, stateKey[st], nil)})
	if s.KeepAwake {
		tip = i18n.T(l, "tray.tooltip.keepawake", map[string]string{"tooltip": tip})
	}
	t.tray.SetTooltip(tip)
	t.tray.SetIcon(Icon(st, s.KeepAwake))
}

var stateKey = map[State]string{
	StateActive:       "tray.state.active",
	StatePaused:       "tray.state.paused",
	StateSending:      "tray.state.sending",
	StateError:        "tray.state.error",
	StateDisconnected: "tray.state.disconnected",
}
