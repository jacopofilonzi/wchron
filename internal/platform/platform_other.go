//go:build !windows

package platform

import "errors"

var errUnsupported = errors.New("not supported on this platform yet")

// NewAutostart is a stub until a Linux/macOS implementation exists.
func NewAutostart(string) Autostart { return unsupportedAutostart{} }

// NewSleepInhibitor is a stub until a Linux/macOS implementation exists.
func NewSleepInhibitor() SleepInhibitor { return unsupportedInhibitor{} }

type unsupportedAutostart struct{}

func (unsupportedAutostart) Enabled() (bool, error) { return false, nil }
func (unsupportedAutostart) Enable() error          { return errUnsupported }
func (unsupportedAutostart) Disable() error         { return nil }

type unsupportedInhibitor struct{}

func (unsupportedInhibitor) Enable() error  { return errUnsupported }
func (unsupportedInhibitor) Disable() error { return nil }
