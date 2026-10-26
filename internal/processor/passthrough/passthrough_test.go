package passthrough

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/processor"
)

func hashOf(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func TestProcessCopiesAndCleansUp(t *testing.T) {
	src := filepath.Join(t.TempDir(), "report.pdf")
	data := bytes.Repeat([]byte("pdf"), 100000)
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(src)
	before := hashOf(t, src)
	tmp := t.TempDir()
	out, cleanup, err := New().Process(context.Background(), processor.Source{Path: src, Name: "report.pdf"}, nil, processor.Env{TmpDir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "report.pdf" || filepath.Dir(out.Path) != tmp || hashOf(t, out.Path) != before {
		t.Fatalf("bad copy: %+v", out)
	}
	cleanup()
	if _, err := os.Stat(out.Path); !os.IsNotExist(err) {
		t.Fatalf("copy still there after cleanup: %v", err)
	}
	after, _ := os.Stat(src)
	if hashOf(t, src) != before || !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("the original changed")
	}
}

func TestProcessFailures(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := New()
	// Missing original: unreadable, nothing written.
	tmp := t.TempDir()
	_, _, err := p.Process(context.Background(), processor.Source{Path: filepath.Join(dir, "gone.txt"), Name: "gone.txt"}, nil, processor.Env{TmpDir: tmp})
	if apperr.As(err).Code != apperr.FileUnreadable {
		t.Fatalf("missing: %v", err)
	}
	// Temp dir that doesn't exist (can't be written): temp.unavailable.
	_, _, err = p.Process(context.Background(), processor.Source{Path: src, Name: "a.txt"}, nil, processor.Env{TmpDir: filepath.Join(tmp, "no", "such")})
	if apperr.As(err).Code != apperr.TempUnavailable {
		t.Fatalf("temp: %v", err)
	}
	// Cancelled run: no copy left.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = p.Process(ctx, processor.Source{Path: src, Name: "a.txt"}, nil, processor.Env{TmpDir: tmp}); err == nil {
		t.Fatal("cancelled copy succeeded")
	}
	if es, _ := os.ReadDir(tmp); len(es) != 0 {
		t.Fatalf("leftovers after failures: %v", es)
	}
}

func TestRegistry(t *testing.T) {
	r := processor.NewRegistry(New())
	if _, err := r.Get("passthrough"); err != nil {
		t.Fatal(err)
	}
	_, err := r.Get("xlsx-sheets-v9")
	if e := apperr.As(err); e.Code != apperr.ProcessorUnknown || e.Params["type"] != "xlsx-sheets-v9" {
		t.Fatalf("got %v", err)
	}
	if s := r.Schemas(); len(s) != 1 || s[0].Type != "passthrough" || s[0].Options == nil {
		t.Fatalf("schemas %+v", s)
	}
}
