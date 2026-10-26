//go:build windows

package platform

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// NewAutostart returns the HKCU Run key autostart for the executable at exe.
func NewAutostart(exe string) Autostart {
	return &runKeyAutostart{open: openRunKey, command: AutostartCommand(exe)}
}

// NewSleepInhibitor returns the SetThreadExecutionState inhibitor.
func NewSleepInhibitor() SleepInhibitor {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")
	return newThreadInhibitor(func(flags uint32) uint32 {
		if err := proc.Find(); err != nil {
			return 0
		}
		r, _, _ := proc.Call(uintptr(flags))
		return uint32(r)
	})
}

type regRunKey struct{ k registry.Key }

func openRunKey() (runKey, func(), error) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil, nil, fmt.Errorf("open HKCU\\%s: %w", runKeyPath, err)
	}
	return regRunKey{k}, func() { _ = k.Close() }, nil
}

func (r regRunKey) get(name string) (string, bool, error) {
	v, _, err := r.k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read Run value %q: %w", name, err)
	}
	return v, true, nil
}

func (r regRunKey) set(name, value string) error {
	if err := r.k.SetStringValue(name, value); err != nil {
		return fmt.Errorf("write Run value %q: %w", name, err)
	}
	return nil
}

func (r regRunKey) del(name string) error {
	if err := r.k.DeleteValue(name); err != nil {
		return fmt.Errorf("delete Run value %q: %w", name, err)
	}
	return nil
}
