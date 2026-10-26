//go:build windows

package platform

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// OpenShared opens path for reading with FILE_SHARE_READ, FILE_SHARE_WRITE
// and FILE_SHARE_DELETE, so a program that has it open (Excel) is never
// disturbed. The error wraps the Windows error (see IsLocked).
func OpenShared(path string) (*os.File, error) {
	return openWith(path, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
}

// OpenExclusive opens path denying any sharing until the returned closer is
// closed: a stand-in for a program holding the file, for tests.
func OpenExclusive(path string) (io.Closer, error) { return openWith(path, 0) }

func openWith(path string, share uint32) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, share, nil, windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// IsLocked reports whether err means another program holds the file
// (sharing or lock violation), as opposed to a missing file or a denied access.
func IsLocked(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
