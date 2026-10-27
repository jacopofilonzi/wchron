package startup

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/logging"
)

// paths is a platform.Paths over a temp dir, with a movable backups dir.
type paths struct{ data, backups string }

func (p paths) DataDir() string     { return p.data }
func (p paths) ConfigFile() string  { return filepath.Join(p.data, "config.json") }
func (p paths) StateFile() string   { return filepath.Join(p.data, "state.json") }
func (p paths) ChannelsDir() string { return filepath.Join(p.data, "channels") }
func (p paths) LogsDir() string     { return filepath.Join(p.data, "logs") }
func (p paths) BackupsDir() string  { return p.backups }
func (p paths) TempDir() string     { return filepath.Join(p.data, "tmp") }
func (p paths) Ensure() error       { return nil }

var now = time.Date(2026, 10, 26, 9, 0, 0, 0, time.Local)

func newPaths(t *testing.T) paths {
	d := t.TempDir()
	return paths{data: d, backups: filepath.Join(d, "backups")}
}

func open(p paths, version string) (Data, error) {
	return Open(Options{Paths: p, AppVersion: version, Now: now, Log: logging.New(io.Discard, 0)})
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const (
	cfgV1   = `{"schema_version": 1, "settings": {"language": "en", "theme": "dark"}}`
	stateV1 = `{"schema_version": 1, "routine_active": false, "last_app_version": "0.1.0"}`
)

func TestFreshInstall(t *testing.T) {
	p := newPaths(t)
	d, err := open(p, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if d.BackupDir != "" || len(d.Warnings) != 0 || !d.State.RoutineActive || d.State.LastAppVersion != "0.2.0" || !d.FirstRun {
		t.Fatalf("%+v", d)
	}
	if !AutostartOnFirstRun(d, false, false) || AutostartOnFirstRun(d, true, false) || AutostartOnFirstRun(d, false, true) {
		t.Fatal("first run autostart: installed copy only")
	}
	if _, err := os.Stat(p.ConfigFile()); !os.IsNotExist(err) {
		t.Fatal("config.json is written on the first save only")
	}
	if !strings.Contains(read(t, p.StateFile()), `"last_app_version": "0.2.0"`) {
		t.Fatal("state.json must record the version")
	}
	if _, err := os.Stat(p.backups); !os.IsNotExist(err) {
		t.Fatal("no backup on a fresh install")
	}
	// Second start, same version: nothing to back up, not a first run any
	// more (state.json exists even if config.json was never written, so a
	// login start the user turned off stays off).
	if d, err := open(p, "0.2.0"); err != nil || d.BackupDir != "" || d.FirstRun || AutostartOnFirstRun(d, false, false) {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestUpgradeBacksUp(t *testing.T) {
	p := newPaths(t)
	put(t, p.ConfigFile(), cfgV1)
	put(t, p.StateFile(), stateV1)
	put(t, filepath.Join(p.ChannelsDir(), "wa-1", "session.db"), "db")
	d, err := open(p, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(d.BackupDir) != "20261026-090000-v0.1.0" {
		t.Fatalf("backup %s", d.BackupDir)
	}
	if read(t, filepath.Join(d.BackupDir, "config.json")) != cfgV1 || read(t, filepath.Join(d.BackupDir, "state.json")) != stateV1 ||
		read(t, filepath.Join(d.BackupDir, "channels", "wa-1", "session.db")) != "db" {
		t.Fatal("backup contents")
	}
	if d.State.RoutineActive || d.State.LastAppVersion != "0.2.0" || d.Config.Settings.Theme != "dark" || d.FirstRun {
		t.Fatalf("%+v", d)
	}
	if read(t, p.ConfigFile()) != cfgV1 {
		t.Fatal("a config at the current version is not rewritten")
	}
}

func TestBackupFailureRefusesStart(t *testing.T) {
	p := newPaths(t)
	put(t, p.ConfigFile(), cfgV1)
	put(t, p.StateFile(), stateV1)
	put(t, p.backups, "a file where the backups folder goes")
	_, err := open(p, "0.2.0")
	if e := apperr.As(err); e.Code != apperr.StorageBackupFailed || e.Params["path"] == "" {
		t.Fatalf("err %v", err)
	}
	if read(t, p.ConfigFile()) != cfgV1 || read(t, p.StateFile()) != stateV1 {
		t.Fatal("the data must be untouched")
	}
	title, msg := FatalText(err, i18n.EN)
	if title == "" || !strings.Contains(msg, p.backups) {
		t.Fatalf("dialog %q %q", title, msg)
	}
}

func TestCorruptState(t *testing.T) {
	p := newPaths(t)
	put(t, p.ConfigFile(), cfgV1)
	put(t, p.StateFile(), `{"routine_active": `)
	d, err := open(p, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Warnings) != 1 || apperr.As(d.Warnings[0]).Code != apperr.StateReadFailed || !d.State.RoutineActive {
		t.Fatalf("%+v", d)
	}
	if read(t, p.StateFile()+".corrupt-20261026-090000") != `{"routine_active": ` {
		t.Fatal("the corrupt file must be kept aside")
	}
	if read(t, filepath.Join(d.BackupDir, "state.json")) != `{"routine_active": ` {
		t.Fatal("the backup holds the corrupt file too")
	}
	if !strings.Contains(read(t, p.StateFile()), `"last_app_version": "0.2.0"`) {
		t.Fatal("a fresh state.json is written")
	}
}

func TestNewerConfigIsFatalAndUntouched(t *testing.T) {
	p := newPaths(t)
	newer := `{"schema_version": 42}`
	put(t, p.ConfigFile(), newer)
	put(t, p.StateFile(), stateV1)
	_, err := open(p, "0.2.0")
	if apperr.As(err).Code != apperr.ConfigNewerVersion {
		t.Fatalf("err %v", err)
	}
	if read(t, p.ConfigFile()) != newer || read(t, p.StateFile()) != stateV1 {
		t.Fatal("files changed")
	}
	if _, err := os.Stat(p.backups); !os.IsNotExist(err) {
		t.Fatal("no backup when refusing")
	}
	title, msg := FatalText(err, i18n.IT)
	if title != "wchron non può avviarsi" || !strings.Contains(msg, "42") {
		t.Fatalf("dialog %q %q", title, msg)
	}
}

// A config.json edited by hand with values that are wrong but structurally
// valid (a bad cadence, an unknown processor, a recipient on a missing
// channel) still starts: the scheduler ignores that schedule with a warning
// (internal/scheduler), the run skips the rest (internal/pipeline), and the
// file is never rewritten. Only structural damage (unknown enum, duplicate
// id, not JSON) stops the start, leaving the file as it is.
func TestHandEditedBadScheduleStillStarts(t *testing.T) {
	p := newPaths(t)
	cfg := `{"schema_version": 1, "settings": {"language": "it", "theme": "system"},
  "channels": [{"id": "wa", "provider": "whatsapp", "name": "Phone", "enabled": true, "settings": {}}],
  "schedules": [
    {"id": "good", "name": "Good", "enabled": true,
     "cadence": {"mode": "fixed", "times": ["09:00"], "days": ["mon"]},
     "recipients": [{"id": "r", "channel_id": "wa", "target": {"kind": "number", "number": "+390000000001"}, "verified": "yes"}],
     "files": [{"id": "f", "path_pattern": "C:/Reports/a.pdf", "match": "error", "processor": "passthrough"}]},
    {"id": "bad", "name": "Bad", "enabled": true,
     "cadence": {"mode": "interval", "interval": {"hours": 0, "minutes": 0}, "anchor": "25:99", "times": ["99:99"], "days": []},
     "recipients": [{"id": "r", "channel_id": "gone", "target": {}, "verified": "unknown"}],
     "files": [{"id": "f", "path_pattern": "relative/x.pdf", "match": "all", "processor": "nope", "processor_options": {"x": 1}}]}
  ]}`
	put(t, p.ConfigFile(), cfg)
	put(t, p.StateFile(), `{"schema_version": 1, "last_app_version": "0.2.0"}`)
	d, err := open(p, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Config.Schedules) != 2 || d.Config.Schedules[1].ID != "bad" || d.FirstRun {
		t.Fatalf("%+v", d.Config.Schedules)
	}
	if read(t, p.ConfigFile()) != cfg {
		t.Fatal("config.json must not be rewritten")
	}
	// Structural damage: the start stops, the file stays as it is.
	broken := strings.Replace(cfg, `"match": "all"`, `"match": "sometimes"`, 1)
	put(t, p.ConfigFile(), broken)
	if _, err := open(p, "0.2.0"); err == nil || read(t, p.ConfigFile()) != broken {
		t.Fatalf("structural damage must stop the start: %v", err)
	}
}
