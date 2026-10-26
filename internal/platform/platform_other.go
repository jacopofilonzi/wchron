//go:build !windows

package platform

import (
	"errors"
	"io"
	"os"
)

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

// FatalDialog is a stub until a Linux/macOS implementation exists: the error
// is only logged.
func FatalDialog(string, string) error { return errUnsupported }

// OpenShared opens path for reading (no sharing modes outside Windows).
func OpenShared(path string) (*os.File, error) { return os.Open(path) }

// OpenExclusive is not supported outside Windows: the tests skip.
func OpenExclusive(string) (io.Closer, error) { return nil, errUnsupported }

// IsLocked is always false outside Windows (advisory locks don't block reads).
func IsLocked(error) bool { return false }
