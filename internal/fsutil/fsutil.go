// Package fsutil holds the file operations shared by the durable data
// packages (config, state, backup).
//
// WriteAtomic is the only way config.json and state.json are written: the new
// content goes to a temp file in the same directory, is flushed to disk and
// then renamed over the target (os.Rename replaces an existing file on
// Windows too). A failure at any step leaves the previous file intact and
// removes the temp file, so a crash or a full disk never leaves a half-written
// data file behind.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// FileMode is the mode of the user data files: owner only. On Windows the
// file inherits the ACL of %APPDATA%, which is already per user.
const FileMode fs.FileMode = 0o600

// DirMode is the mode of the data directories.
const DirMode fs.FileMode = 0o700

// Seams for the tests: simulate a failure after the temp file is written.
var (
	rename   = os.Rename
	syncFile = func(f *os.File) error { return f.Sync() }
)

// WriteAtomic replaces path with data (temp file + fsync + rename). The temp
// file is created with mode 0600 by os.CreateTemp, and the rename keeps it.
func WriteAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err = syncFile(tmp); err != nil {
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err = rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// Exists reports whether path exists; any error other than "not found" is
// returned, so an unreadable path is never mistaken for a missing one.
func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", path, err)
}

// CopyFile copies src to dst (created or truncated) and flushes it to disk.
func CopyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, FileMode)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", dst, cerr)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	if err = out.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dst, err)
	}
	return nil
}

// CopyTree copies every regular file under src into dst, keeping the layout.
// Symlinks and other special files are skipped: the data dir never holds them.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relative path of %s: %w", path, err)
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			if err := os.MkdirAll(target, DirMode); err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			return nil
		case d.Type().IsRegular():
			return CopyFile(path, target)
		default:
			return nil
		}
	})
}
