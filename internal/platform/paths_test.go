package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

func TestPathsDefaultAndOverride(t *testing.T) {
	cfg, tmp, cwd := filepath.FromSlash("/u/AppData/Roaming"), filepath.FromSlash("/u/Temp"), filepath.FromSlash("/work")
	p := newPaths("", cfg, tmp, cwd)
	if p.DataDir() != filepath.Join(cfg, "wchron") || p.TempDir() != filepath.Join(tmp, "wchron") {
		t.Fatalf("default: %+v", p)
	}
	if p.ConfigFile() != filepath.Join(cfg, "wchron", "config.json") || p.BackupsDir() != filepath.Join(cfg, "wchron", "backups") {
		t.Fatalf("files: %s %s", p.ConfigFile(), p.BackupsDir())
	}
	if got := newPaths("./.devdata", cfg, tmp, cwd).DataDir(); got != filepath.Join(cwd, ".devdata") {
		t.Fatalf("relative override: %s", got)
	}
	abs := filepath.Join(t.TempDir(), "data")
	if got := newPaths(abs, cfg, tmp, cwd).DataDir(); got != abs {
		t.Fatalf("absolute override: %s", got)
	}
}

func TestEnsure(t *testing.T) {
	root := t.TempDir()
	p := newPaths(filepath.Join(root, "data"), "", filepath.Join(root, "tmp"), root)
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{p.DataDir(), p.ChannelsDir(), p.LogsDir(), p.BackupsDir(), p.TempDir()} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Fatalf("%s: %v", d, err)
		}
	}
	entries, _ := os.ReadDir(p.DataDir())
	if len(entries) != 3 {
		t.Fatalf("the write check must leave nothing behind: %v", entries)
	}
}

func TestEnsureUnavailable(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p := newPaths(filepath.Join(file, "data"), "", root, root)
	if e := apperr.As(p.Ensure()); e.Code != apperr.StorageUnavailable || e.Params["path"] == "" {
		t.Fatalf("err %v", e)
	}
}
