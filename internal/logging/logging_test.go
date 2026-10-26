package logging

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

func TestMaskPhone(t *testing.T) {
	cases := map[string]string{
		"+393331234567":   "***567",
		"+39 333 1234567": "***567",
		"123":             "***",
		"":                "***",
		"12a34":           "***234",
	}
	for in, want := range cases {
		if got := MaskPhone(in); got != want {
			t.Errorf("MaskPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenWritesJSONAndStderr(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer
	log, closer := Open(Options{Dir: dir, Level: slog.LevelInfo, Stderr: &stderr})
	log.Debug("hidden")
	log.Info("started", "version", "0.2.0")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := Read(dir, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Msg != "started" || entries[0].Level != "INFO" || entries[0].Attrs["version"] != "0.2.0" {
		t.Fatalf("entries %+v", entries)
	}
	if !strings.Contains(stderr.String(), "msg=started") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func line(t time.Time, level, msg string, n int) string {
	return fmt.Sprintf(`{"time":%q,"level":%q,"msg":%q,"n":%d}`+"\n", t.Format(time.RFC3339Nano), level, msg, n)
}

// The reader goes through the current file first, then the rotated ones from
// the newest, and skips lines that are not records.
func TestReadAcrossRotatedFiles(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	n := 0
	write := func(name string, count int, extra string) {
		var b strings.Builder
		for range count {
			level := "INFO"
			if n%3 == 0 {
				level = "ERROR"
			}
			b.WriteString(line(base.Add(time.Duration(n)*time.Minute), level, "m", n))
			n++
		}
		b.WriteString(extra)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("wchron-2026-10-24T08-00-00.000.log", 4, "")
	write("wchron-2026-10-25T08-00-00.000.log", 4, "not json\n")
	write("wchron.log", 4, `{"time":"2026-10-26T`) // cut by a crash
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte(line(base, "ERROR", "x", 99)), 0o600); err != nil {
		t.Fatal(err)
	}

	all, err := Read(dir, Filter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 12 {
		t.Fatalf("got %d entries", len(all))
	}
	for i := range all {
		if want := float64(11 - i); all[i].Attrs["n"] != want {
			t.Fatalf("entry %d: n=%v, want %v", i, all[i].Attrs["n"], want)
		}
	}
	some, err := Read(dir, Filter{Limit: 6})
	if err != nil || len(some) != 6 || some[5].Attrs["n"] != float64(6) {
		t.Fatalf("limit: %+v %v", some, err)
	}
	errs, err := Read(dir, Filter{Level: "error"})
	if err != nil || len(errs) != 4 {
		t.Fatalf("errors only: %d %v", len(errs), err)
	}
	for _, e := range errs {
		if e.Level != "ERROR" {
			t.Fatalf("level %s", e.Level)
		}
	}
}

func TestReadMissingAndInvalid(t *testing.T) {
	if e, err := Read(filepath.Join(t.TempDir(), "none"), Filter{}); err != nil || len(e) != 0 || e == nil {
		t.Fatalf("missing dir: %v %v", e, err)
	}
	if _, err := Read(t.TempDir(), Filter{Level: "loud"}); apperr.As(err).Code != apperr.Internal {
		t.Fatalf("err %v", err)
	}
	// Folders named like a log file are ignored.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "wchron-2026-01-01T00-00-00.000.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if e, err := Read(dir, Filter{}); err != nil || len(e) != 0 {
		t.Fatalf("folder: %v %v", e, err)
	}
}
