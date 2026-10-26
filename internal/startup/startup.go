// Package startup opens the durable data at launch, in the order that keeps
// it safe (see AGENTS.md "Upgrading never loses data"):
//
//  1. load config.json and state.json in memory, migrating them in memory
//     only (a newer schema, an unreadable config.json or a failed migration
//     stops here: nothing has been written);
//  2. back up the data when backup.Needed says so: a failure stops the start
//     (storage.backup_failed) with the data files untouched; old backups
//     beyond backup.Keep are pruned (a pruning failure is only logged);
//  3. move a corrupt state.json aside (state.json.corrupt-<timestamp>);
//  4. save a migrated config.json (a failure stops the start);
//  5. save state.json with last_app_version set to this version (a failure
//     is only a warning: state.write_failed).
//
// Data.FirstRun marks the very first start (no config.json, no state.json):
// main then turns on the login start (AutostartOnFirstRun).
//
// It runs after platform.Paths.Ensure and before any channel is opened, so
// the backup is a plain copy of closed files. main shows a fatal error with
// FatalText and exits without touching the data.
package startup

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/backup"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/platform"
	"github.com/jacopofilonzi/wchron/internal/state"
)

// Data is the loaded data, ready for the app.
type Data struct {
	Config config.Config
	State  state.State
	// BackupDir is the backup made at this start ("" when none was needed).
	BackupDir string
	// Warnings are the non-fatal problems met (state.read_failed,
	// state.write_failed), already logged.
	Warnings []error
	// FirstRun: neither config.json nor state.json existed, so this is the
	// very first start in this data dir (state.json is written at every
	// start, config.json only on the first change).
	FirstRun bool
}

// AutostartOnFirstRun reports whether this start should turn on the login
// start by itself: only on the first run of an installed copy, never for a
// dev build or a dev run with its own data dir (WCHRON_DATA_DIR), so a dev
// machine's Run key is never touched behind the developer's back.
func AutostartOnFirstRun(d Data, devBuild, dataDirOverridden bool) bool {
	return d.FirstRun && !devBuild && !dataDirOverridden
}

// Options are the inputs of Open.
type Options struct {
	Paths      platform.Paths
	AppVersion string
	Now        time.Time
	Log        *slog.Logger
}

// Open loads, backs up and migrates the data (see the package doc). Any
// error it returns is fatal and is an apperr.Error.
func Open(o Options) (Data, error) {
	p := o.Paths
	cl, err := config.Load(p.ConfigFile())
	if err != nil {
		return Data{}, err
	}
	sl, err := state.Load(p.StateFile())
	if err != nil {
		return Data{}, err
	}
	d := Data{FirstRun: !cl.Exists && !sl.Exists}
	last := sl.State.LastAppVersion
	if backup.Needed(backup.Inputs{
		ConfigExists:   cl.Exists,
		StateExists:    sl.Exists,
		ConfigMigrated: cl.Migrated,
		StateMigrated:  sl.Migrated,
		LastAppVersion: last,
		AppVersion:     o.AppVersion,
	}) {
		dir, err := backup.Create(p.DataDir(), p.BackupsDir(), last, o.Now)
		if err != nil {
			return Data{}, apperr.New(apperr.StorageBackupFailed, fmt.Errorf("backup: %w", err)).With("path", dir)
		}
		d.BackupDir = dir
		o.Log.Info("data backed up", "dir", dir, "from_version", last, "to_version", o.AppVersion,
			"config_from_schema", cl.From, "state_from_schema", sl.From)
		removed, err := backup.Prune(p.BackupsDir(), backup.Keep)
		if err != nil {
			o.Log.Warn("prune old backups", "err", err)
		}
		for _, r := range removed {
			o.Log.Info("old backup removed", "dir", r)
		}
	}

	if sl.Corrupt != nil {
		d.Warnings = append(d.Warnings, sl.Corrupt)
		if dst, err := state.Quarantine(p.StateFile(), o.Now); err != nil {
			o.Log.Warn("state unreadable, could not move it aside", "err", errors.Join(sl.Corrupt, err))
		} else {
			o.Log.Warn("state unreadable, starting from defaults", "err", sl.Corrupt, "kept_as", dst)
		}
	}

	if cl.Migrated {
		if err := config.Save(p.ConfigFile(), cl.Config); err != nil {
			return Data{}, err
		}
		o.Log.Info("config migrated", "from", cl.From, "to", config.CurrentVersion)
	}

	st := sl.State
	if !sl.Exists || sl.Migrated || sl.Corrupt != nil || st.LastAppVersion != o.AppVersion {
		st.LastAppVersion = o.AppVersion
		if err := state.Save(p.StateFile(), st); err != nil {
			d.Warnings = append(d.Warnings, err)
			o.Log.Warn("save state", "err", err)
		}
	}
	d.Config, d.State = cl.Config, st
	return d, nil
}

// FatalText is the title and message of the startup error dialog for err,
// in lang (the text of the error code, never the Go message).
func FatalText(err error, lang i18n.Lang) (title, message string) {
	e := apperr.As(err)
	return i18n.T(lang, "dialog.fatal.title", nil), i18n.T(lang, "error."+string(e.Code), e.Params)
}
