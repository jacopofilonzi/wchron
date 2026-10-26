package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicReplaces(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := WriteAtomic(p, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(p, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "two" {
		t.Fatalf("got %q", b)
	}
	assertOnlyFile(t, filepath.Dir(p), "config.json")
}

func TestWriteAtomicFailureKeepsOldFile(t *testing.T) {
	for name, fail := range map[string]func(){
		"rename": func() { rename = func(string, string) error { return errors.New("disk full") } },
		"sync":   func() { syncFile = func(*os.File) error { return errors.New("io error") } },
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "state.json")
			if err := WriteAtomic(p, []byte("old")); err != nil {
				t.Fatal(err)
			}
			origRename, origSync := rename, syncFile
			t.Cleanup(func() { rename, syncFile = origRename, origSync })
			fail()
			if err := WriteAtomic(p, []byte("new")); err == nil {
				t.Fatal("expected an error")
			}
			if b, _ := os.ReadFile(p); string(b) != "old" {
				t.Fatalf("old file changed: %q", b)
			}
			assertOnlyFile(t, filepath.Dir(p), "state.json")
		})
	}
}

func TestWriteAtomicMissingDir(t *testing.T) {
	if err := WriteAtomic(filepath.Join(t.TempDir(), "nope", "x.json"), nil); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCopyTree(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")
	files := map[string]string{
		filepath.Join("a", "session.db"):     "db",
		filepath.Join("a", "session.db-wal"): "wal",
		filepath.Join("b", "c", "secret"):    "s",
	}
	for rel, content := range files {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for rel, content := range files {
		if b, err := os.ReadFile(filepath.Join(dst, rel)); err != nil || string(b) != content {
			t.Fatalf("%s: %q %v", rel, b, err)
		}
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	if ok, err := Exists(dir); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if ok, err := Exists(filepath.Join(dir, "x")); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func assertOnlyFile(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dir holds %v, want only %s", names, name)
	}
}
