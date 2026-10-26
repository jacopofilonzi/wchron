package platform

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSharedAndLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Another program writing the file must not stop a shared read.
	w, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	f, err := OpenShared(path)
	if err != nil {
		t.Fatalf("shared open while another handle writes: %v", err)
	}
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "data" {
		t.Fatalf("read %q, %v", b, err)
	}
	_ = f.Close()
	_ = w.Close()

	lock, err := OpenExclusive(path)
	if err != nil {
		t.Skipf("no exclusive open on this platform: %v", err)
	}
	defer lock.Close()
	if _, err := OpenShared(path); err == nil || !IsLocked(err) {
		t.Fatalf("open of an exclusively held file: %v, want a lock error", err)
	}
	if _, err := OpenShared(filepath.Join(t.TempDir(), "missing")); err == nil || IsLocked(err) {
		t.Fatalf("missing file: %v", err)
	}
}
