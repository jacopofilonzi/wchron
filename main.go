// Command wchron is the tray app: one process holding the state, the tray and
// the Wails window.
//
// Startup order (see STRUCTURE.md): Wails application.New first, because it
// exits a second instance right away (after notifying the first one), so
// nothing with side effects may run before it. Then: dev env overrides →
// data dir paths → log file → config.json and state.json (startup.Open:
// load, backup, migrate, save) → Store → channel manager, service, window,
// tray → Run; the scheduler and the channels start when the application has
// started; on quit the scheduler stops first, then the channels close. A
// fatal error shows a native error dialog in the user's language and exits
// with code 1 without touching the data.
package main

import (
	"embed"
	"io"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/jacopofilonzi/wchron/internal/app"
	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/devenv"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/platform"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/provider/whatsapp"
	"github.com/jacopofilonzi/wchron/internal/scheduler"
	"github.com/jacopofilonzi/wchron/internal/startup"
	"github.com/jacopofilonzi/wchron/internal/state"
	"github.com/jacopofilonzi/wchron/internal/tray"
	"github.com/jacopofilonzi/wchron/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Until the data dir is known, logs go to stderr only.
	log := logging.New(os.Stderr, slog.LevelInfo)
	// A plain scan instead of the flag package: Wails and Windows may pass
	// arguments of their own, which must not make the app exit.
	minimized := slices.Contains(os.Args[1:], platform.MinimizedFlag)

	var (
		window  *application.WebviewWindow
		store   *app.Store
		sched   *scheduler.Scheduler
		chans   *provider.Manager
		logFile io.Closer
	)
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

	wails := application.New(application.Options{
		Name:         "wchron",
		Description:  "Scheduled file sending to WhatsApp and other channels",
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
			if sched != nil && !sched.Stop(5*time.Second) {
				log.Warn("quit with runs still in progress")
			}
			if chans != nil {
				chans.Close()
			}
			if store != nil {
				if err := store.Shutdown(); err != nil {
					log.Error("shutdown", "err", err)
				}
			}
			log.Info("stopped")
			if logFile != nil {
				_ = logFile.Close()
			}
		},
	})
	// From here on this is the only instance.

	fatal := func(err error, lang i18n.Lang) {
		log.Error("cannot start", "err", err)
		title, msg := startup.FatalText(err, lang)
		if derr := platform.FatalDialog(title, msg); derr != nil {
			log.Error("fatal dialog", "err", derr)
		}
		if logFile != nil {
			_ = logFile.Close()
		}
		os.Exit(1)
	}

	env, err := devenv.Load(os.LookupEnv)
	if err != nil {
		fatal(err, i18n.Default)
	}
	paths, err := platform.NewPaths(env.DataDir)
	if err != nil {
		fatal(err, i18n.Default)
	}
	if err := paths.Ensure(); err != nil {
		fatal(err, config.PeekLanguage(paths.ConfigFile()))
	}
	var stderr io.Writer
	if version.Dev {
		stderr = os.Stderr
	}
	log, logFile = logging.Open(logging.Options{Dir: paths.LogsDir(), Level: env.LogLevel, Stderr: stderr})
	wails.Logger = log
	log.Info("starting", "version", version.Version, "minimized", minimized, "data_dir", paths.DataDir(), "dev", version.Dev)
	if err := i18n.LoadError(); err != nil {
		log.Error("i18n catalog", "err", err)
	}

	data, err := startup.Open(startup.Options{Paths: paths, AppVersion: version.Version, Now: time.Now(), Log: log})
	if err != nil {
		fatal(err, config.PeekLanguage(paths.ConfigFile()))
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

	store = app.NewStore(app.Deps{
		Inhibitor:   inhibitor,
		Autostart:   autostart,
		AutostartOn: autostartOn,
		Config:      data.Config,
		State:       data.State,
		SaveConfig:  func(c config.Config) error { return config.Save(paths.ConfigFile(), c) },
		SaveState:   func(s state.State) error { return state.Save(paths.StateFile(), s) },
		Log:         log,
	})
	if err := store.Start(); err != nil {
		log.Warn("keep awake at startup", "err", err)
	}
	// The providers: the only place that names them (AGENTS.md "Adding a provider").
	registry := provider.NewRegistry(whatsapp.New())
	chans = provider.NewManager(provider.Options{
		Registry:   registry,
		Dir:        paths.ChannelsDir(),
		Store:      store,
		Log:        log,
		Clock:      clock.Real{},
		AppVersion: version.Version,
		OnChange:   store.SetChannels,
		OnStatus:   func(c provider.ChannelInfo) { wails.Event.Emit(app.EventChannelStatus, c) },
		OnPairing:  func(p provider.PairingEvent) { wails.Event.Emit(app.EventChannelQR, p) },
		OnSchedulesChanged: func() {
			if sched != nil {
				sched.Reload(store.Schedules())
			}
		},
		// The update check arrives with milestone 10.
		OnOutdated: func(id string) { log.Warn("channel rejected as outdated: an update is needed", "channel", id) },
	})
	wails.RegisterService(application.NewService(app.NewService(store, registry, chans, paths.LogsDir(), clock.Real{})))

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

	// No pipeline yet (milestone 6): runs are only logged (DryRunner).
	sched = scheduler.New(scheduler.Options{
		Clock:    clock.Real{},
		Location: time.Local,
		Runner:   scheduler.DryRunner{Clock: clock.Real{}, Log: log},
		Store:    store,
		Log:      log,
		OnNext: func(n *scheduler.NextRun) {
			if n == nil {
				store.SetNextRun(nil)
				return
			}
			store.SetNextRun(&app.NextRun{ScheduleID: n.ScheduleID, ScheduleName: n.ScheduleName, At: n.At})
		},
	})
	wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		sched.Reload(store.Schedules())
		sched.Start()
		// After the startup backup (startup.Open), never before: it copies
		// channels/ as plain files.
		chans.Start()
	})

	if err := wails.Run(); err != nil {
		log.Error("run", "err", err)
		os.Exit(1)
	}
}
