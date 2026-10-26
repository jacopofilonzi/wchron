// Command wchron is the tray app: one process holding the state, the tray and
// the Wails window (milestone 1: no persistence yet, see STRUCTURE.md).
package main

import (
	"embed"
	"log/slog"
	"os"
	"slices"

	"github.com/jacopofilonzi/wchron/internal/app"
	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/platform"
	"github.com/jacopofilonzi/wchron/internal/tray"
	"github.com/jacopofilonzi/wchron/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	log := logging.New(os.Stderr, slog.LevelInfo)
	if err := i18n.LoadError(); err != nil {
		log.Error("i18n catalog", "err", err)
	}
	// A plain scan instead of the flag package: Wails and Windows may pass
	// arguments of their own, which must not make the app exit.
	minimized := slices.Contains(os.Args[1:], platform.MinimizedFlag)

	var window *application.WebviewWindow
	showWindow := func() {
		if window == nil {
			return
		}
		if window.IsMinimised() {
			window.Restore()
		}
		window.Show()
		window.Focus()
	}

	inhibitor := platform.NewSleepInhibitor()
	exe, err := os.Executable()
	if err != nil {
		log.Error("executable path, autostart disabled", "err", err)
	}
	autostart := platform.NewAutostart(exe)
	// Reading the Run key has no side effect; it seeds the Settings toggle.
	autostartOn := false
	if exe != "" {
		if autostartOn, err = autostart.Enabled(); err != nil {
			log.Warn("read autostart", "err", err)
		}
	}

	// application.New exits a second instance right away (after notifying the
	// first one), so nothing with side effects may run before it.
	store := app.NewStore(inhibitor, autostart, autostartOn)
	wails := application.New(application.Options{
		Name:         "wchron",
		Description:  "Scheduled file sending to WhatsApp",
		Services:     []application.Service{application.NewService(app.NewService(store))},
		Assets:       application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Logger:       log,
		LogLevel:     slog.LevelWarn,
		MarshalError: apperr.MarshalForFrontend,
		Windows:      application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.jacopofilonzi.wchron",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				log.Info("second launch: focusing the window")
				application.InvokeAsync(showWindow)
			},
		},
		OnShutdown: func() {
			if err := store.Shutdown(); err != nil {
				log.Error("shutdown", "err", err)
			}
		},
	})

	window = wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "wchron",
		Width:            1120,
		Height:           720,
		MinWidth:         900,
		MinHeight:        600,
		Hidden:           minimized,
		BackgroundColour: application.NewRGB(0xF7, 0xF7, 0xF8),
		URL:              "/",
	})
	// Closing the window only hides it: the app keeps running in the tray.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
	})

	store.Subscribe(func(s app.Snapshot) { wails.Event.Emit(app.EventState, s) })
	tray.New(wails, store, log, showWindow)

	log.Info("starting", "version", version.Version, "minimized", minimized)
	if err := wails.Run(); err != nil {
		log.Error("run", "err", err)
		os.Exit(1)
	}
}
