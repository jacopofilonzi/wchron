package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNeeded(t *testing.T) {
	cases := []struct {
		name string
		in   Inputs
		want bool
	}{
		{"fresh install", Inputs{AppVersion: "0.2.0"}, false},
		{"same version", Inputs{ConfigExists: true, StateExists: true, LastAppVersion: "0.2.0", AppVersion: "0.2.0"}, false},
		{"only state, same version", Inputs{StateExists: true, LastAppVersion: "0.2.0", AppVersion: "0.2.0"}, false},
		{"upgrade", Inputs{ConfigExists: true, StateExists: true, LastAppVersion: "0.1.0", AppVersion: "0.2.0"}, true},
		{"downgrade", Inputs{ConfigExists: true, StateExists: true, LastAppVersion: "0.3.0", AppVersion: "0.2.0"}, true},
		{"config migration", Inputs{ConfigExists: true, StateExists: true, ConfigMigrated: true, LastAppVersion: "0.2.0", AppVersion: "0.2.0"}, true},
		{"state migration", Inputs{ConfigExists: true, StateExists: true, StateMigrated: true, LastAppVersion: "0.2.0", AppVersion: "0.2.0"}, true},
		{"config without state", Inputs{ConfigExists: true, AppVersion: "0.2.0"}, true},
		{"unknown last version", Inputs{ConfigExists: true, StateExists: true, AppVersion: "0.2.0"}, true},
	}
	for _, c := range cases {
		if got := Needed(c.in); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
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

var now = time.Date(2026, 10, 26, 9, 5, 7, 0, time.Local)

func TestCreateCopiesEverything(t *testing.T) {
	data := t.TempDir()
	files := map[string]string{
		"config.json": `{"schema_version":1}`,
		"state.json":  `{"schema_version":1}`,
		filepath.Join("channels", "wa-1", "session.db"):     "sqlite",
		filepath.Join("channels", "wa-1", "session.db-wal"): "wal",
		filepath.Join("channels", "wa-1", "session.db-shm"): "shm",
		filepath.Join("channels", "tg-1", "secret.bin"):     "dpapi",
	}
	for rel, c := range files {
		put(t, filepath.Join(data, rel), c)
	}
	put(t, filepath.Join(data, "logs", "wchron.log"), "not backed up")
	backups := filepath.Join(data, "backups")
	dst, err := Create(data, backups, "0.1.0", now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dst) != "20261026-090507-v0.1.0" {
		t.Fatalf("dst %s", dst)
	}
	for rel, c := range files {
		if b, err := os.ReadFile(filepath.Join(dst, rel)); err != nil || string(b) != c {
			t.Errorf("%s: %q %v", rel, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "logs")); !os.IsNotExist(err) {
		t.Error("logs must not be backed up")
	}
	// Same second, unknown version: a new folder, not a merge.
	dst2, err := Create(data, backups, "../../evil", now)
	if err != nil || filepath.Base(dst2) != "20261026-090507-vunknown" {
		t.Fatalf("dst2 %s %v", dst2, err)
	}
	dst3, err := Create(data, backups, "unknown", now)
	if err != nil || filepath.Base(dst3) != "20261026-090507-vunknown-2" {
		t.Fatalf("dst3 %s %v", dst3, err)
	}
}

func TestCreateOnlyConfig(t *testing.T) {
	data := t.TempDir()
	put(t, filepath.Join(data, "config.json"), "{}")
	dst, err := Create(data, filepath.Join(data, "backups"), "", now)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Fatalf("entries %v", entries)
	}
}

func TestCreateFailure(t *testing.T) {
	data := t.TempDir()
	put(t, filepath.Join(data, "config.json"), "{}")
	// backups is a file: the folder can't be created.
	blocked := filepath.Join(data, "backups")
	put(t, blocked, "")
	if _, err := Create(data, blocked, "0.1.0", now); err == nil {
		t.Fatal("expected an error")
	}
	// A data file that can't be copied (here a folder named state.json):
	// the error is returned and the partial folder is removed.
	data2 := t.TempDir()
	put(t, filepath.Join(data2, "config.json"), "{}")
	if err := os.Mkdir(filepath.Join(data2, "state.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(t.TempDir(), "backups")
	dst, err := Create(data2, backups, "0.1.0", now)
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, serr := os.Stat(dst); !os.IsNotExist(serr) {
		t.Fatalf("partial backup left at %s", dst)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	for i := range 13 {
		if err := os.Mkdir(filepath.Join(dir, start.Add(time.Duration(i)*time.Hour).Format("20060102-150405")+"-v0.1.0"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "my notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	removed, err := Prune(dir, Keep)
	if err != nil || len(removed) != 3 {
		t.Fatalf("removed %v %v", removed, err)
	}
	for _, r := range removed {
		if b := filepath.Base(r); b >= "20260101-030000" {
			t.Errorf("removed a recent backup %s", b)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != Keep+1 {
		t.Fatalf("left %d entries", len(entries))
	}
	if removed, err := Prune(dir, Keep); err != nil || removed != nil {
		t.Fatalf("second prune: %v %v", removed, err)
	}
}
