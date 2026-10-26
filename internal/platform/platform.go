// Package platform is the only place with OS-specific code. The rest of the
// app depends on the interfaces declared here; the Windows implementations live
// in `_windows.go` files and `_other.go` holds stubs so every package still
// compiles (and `go vet` runs) on Linux and macOS.
//
// Surface: SleepInhibitor, Autostart, Paths (data dir layout, paths.go) and
// FatalDialog (the startup error box, MessageBoxW) are implemented; Notifier
// (desktop toast) and Protect/Unprotect (DPAPI) come later. Single instance is
// handled by Wails (application.Options.SingleInstance), not here.
//
// Keep awake on Windows: SetThreadExecutionState(ES_CONTINUOUS |
// ES_SYSTEM_REQUIRED) keeps the system awake, not necessarily the display.
// The state belongs to the calling OS thread and lasts until that thread calls
// again or exits, while goroutines migrate between threads: every call is made
// from one dedicated goroutine locked to its OS thread (see threadInhibitor).
// Future: logind/D-Bus inhibitor on Linux, IOKit on macOS.
//
// Autostart on Windows: value "wchron" under
// HKCU\Software\Microsoft\Windows\CurrentVersion\Run, set to the quoted exe
// path plus MinimizedFlag, so a login start goes straight to the tray. Per user,
// no elevation.
//
// Unit tests never touch the real registry or power state: the logic is tested
// through the runKey and executionStateFunc seams with fakes.
package platform

import "strings"

// MinimizedFlag starts the app hidden in the tray (used by autostart).
const MinimizedFlag = "--minimized"

// Autostart controls whether the app starts at login.
type Autostart interface {
	// Enabled reports whether login start is set for this executable.
	Enabled() (bool, error)
	Enable() error
	Disable() error
}

// SleepInhibitor keeps the system from sleeping while enabled.
type SleepInhibitor interface {
	Enable() error
	Disable() error
}

// autostartValue is the Run key value name.
const autostartValue = "wchron"

// AutostartCommand is the command line stored for login start: the quoted
// executable path (it usually contains spaces) followed by MinimizedFlag.
func AutostartCommand(exe string) string {
	return `"` + exe + `" ` + MinimizedFlag
}

// runKey is the subset of a registry key used by autostart, faked in tests.
type runKey interface {
	// get returns the value, ok=false when it does not exist.
	get(name string) (value string, ok bool, err error)
	set(name, value string) error
	del(name string) error
}

// runKeyAutostart implements Autostart over a Run key.
type runKeyAutostart struct {
	open    func() (runKey, func(), error)
	command string
}

func (a *runKeyAutostart) Enabled() (bool, error) {
	k, done, err := a.open()
	if err != nil {
		return false, err
	}
	defer done()
	v, ok, err := k.get(autostartValue)
	if err != nil || !ok {
		return false, err
	}
	// Another path (e.g. an old install location) does not start this exe.
	return strings.EqualFold(v, a.command), nil
}

func (a *runKeyAutostart) Enable() error {
	k, done, err := a.open()
	if err != nil {
		return err
	}
	defer done()
	return k.set(autostartValue, a.command)
}

func (a *runKeyAutostart) Disable() error {
	k, done, err := a.open()
	if err != nil {
		return err
	}
	defer done()
	_, ok, err := k.get(autostartValue)
	if err != nil || !ok {
		return err
	}
	return k.del(autostartValue)
}
