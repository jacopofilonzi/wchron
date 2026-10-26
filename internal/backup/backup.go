// Package backup copies the durable data before it can change shape.
//
// When (Needed): at startup, after config.json and state.json are loaded in
// memory but before anything is written and before any channel (and its
// SQLite database) is opened, if a migration is pending, if the app version
// differs from state.json's last_app_version (upgrade or downgrade, or a
// corrupt state.json), or if config.json exists without state.json. A fresh
// install (neither file) has nothing to back up.
//
// What (Create): config.json, state.json and the whole channels/ tree,
// copied to backups/<YYYYMMDD-HHMMSS>-v<old version>/ with the same layout.
// A plain file copy is consistent because nothing is open yet; SQLite side
// files (session.db-wal, -shm) are copied with their database. A failed copy
// removes the partial folder and the caller refuses to start
// (storage.backup_failed).
//
// Retention (Prune): the Keep most recent backup folders are kept, older ones
// are deleted. They are app-generated copies, not user data, so this is not
// covered by the "never delete user data" rule; folders that don't look like
// backups are never touched.
package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/jacopofilonzi/wchron/internal/fsutil"
)

// Keep is the number of backups retained.
const Keep = 10

// Inputs is what Needed decides on.
type Inputs struct {
	ConfigExists, StateExists     bool
	ConfigMigrated, StateMigrated bool
	// LastAppVersion is state.json's last_app_version ("" if unknown).
	LastAppVersion string
	AppVersion     string
}

// Needed reports whether the data must be backed up before starting.
func Needed(in Inputs) bool {
	switch {
	case !in.ConfigExists && !in.StateExists:
		return false
	case in.ConfigMigrated || in.StateMigrated:
		return true
	case !in.StateExists:
		return true
	default:
		return in.LastAppVersion != in.AppVersion
	}
}

var (
	safeVersion = regexp.MustCompile(`^[0-9A-Za-z.+-]{1,32}$`)
	backupDir   = regexp.MustCompile(`^\d{8}-\d{6}-v`)
)

// Files names the data to copy, relative to the data dir.
var Files = []string{"config.json", "state.json"}

// ChannelsDir is the channels tree, relative to the data dir.
const ChannelsDir = "channels"

// Create copies the data of dataDir into a new folder of backupsDir and
// returns its path. oldVersion labels the folder ("unknown" when empty or
// not a plain version string, since it comes from a file).
func Create(dataDir, backupsDir, oldVersion string, now time.Time) (string, error) {
	if !safeVersion.MatchString(oldVersion) {
		oldVersion = "unknown"
	}
	if err := os.MkdirAll(backupsDir, fsutil.DirMode); err != nil {
		return backupsDir, fmt.Errorf("create %s: %w", backupsDir, err)
	}
	base := now.Format("20060102-150405") + "-v" + oldVersion
	dst := filepath.Join(backupsDir, base)
	for i := 2; ; i++ {
		err := os.Mkdir(dst, fsutil.DirMode)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) || i > 99 {
			return dst, fmt.Errorf("create %s: %w", dst, err)
		}
		dst = filepath.Join(backupsDir, base+"-"+strconv.Itoa(i))
	}
	if err := copyData(dataDir, dst); err != nil {
		if rerr := os.RemoveAll(dst); rerr != nil {
			err = errors.Join(err, fmt.Errorf("remove partial backup: %w", rerr))
		}
		return dst, err
	}
	return dst, nil
}

func copyData(dataDir, dst string) error {
	for _, name := range Files {
		src := filepath.Join(dataDir, name)
		ok, err := fsutil.Exists(src)
		if err != nil {
			return err
		}
		if ok {
			if err := fsutil.CopyFile(src, filepath.Join(dst, name)); err != nil {
				return err
			}
		}
	}
	src := filepath.Join(dataDir, ChannelsDir)
	ok, err := fsutil.Exists(src)
	if err != nil || !ok {
		return err
	}
	return fsutil.CopyTree(src, filepath.Join(dst, ChannelsDir))
}

// Prune deletes the oldest backup folders beyond keep and returns the
// removed paths. Folder names start with the timestamp, so they sort by age.
func Prune(backupsDir string, keep int) ([]string, error) {
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", backupsDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && backupDir.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return nil, nil
	}
	slices.Sort(names)
	var removed []string
	var errs []error
	for _, n := range names[:len(names)-keep] {
		p := filepath.Join(backupsDir, n)
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", p, err))
			continue
		}
		removed = append(removed, p)
	}
	return removed, errors.Join(errs...)
}

// Latest returns the time of the most recent backup in backupsDir (from its
// folder name, a wall-clock time in loc: Create names folders with the local
// time), or ok false when there is none.
func Latest(backupsDir string, loc *time.Location) (t time.Time, ok bool, err error) {
	entries, err := os.ReadDir(backupsDir)
	if errors.Is(err, os.ErrNotExist) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("list %s: %w", backupsDir, err)
	}
	var last string
	for _, e := range entries {
		if e.IsDir() && backupDir.MatchString(e.Name()) && e.Name() > last {
			last = e.Name()
		}
	}
	if last == "" {
		return time.Time{}, false, nil
	}
	t, err = time.ParseInLocation("20060102-150405", last[:len("20060102-150405")], loc)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("backup folder %s: %w", last, err)
	}
	return t, true, nil
}
