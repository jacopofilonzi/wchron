package resolver

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/platform"
)

var slot = time.Date(2026, 10, 26, 14, 0, 0, 0, time.Local)

func write(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func code(err error) apperr.Code {
	if err == nil {
		return ""
	}
	return apperr.As(err).Code
}

func names(ms []Match) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Name)
	}
	return out
}

func TestResolveTokensAtSlot(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "report 26-10-2026.xlsx"), slot)
	write(t, filepath.Join(dir, "report 25-10-2026.xlsx"), slot)
	spec := config.FileSpec{PathPattern: filepath.Join(dir, "report {date:DD-MM-YYYY}.xlsx"), Match: config.MatchError}
	ms, err := Resolve(spec, slot, i18n.IT)
	if err != nil || len(ms) != 1 || ms[0].Name != "report 26-10-2026.xlsx" {
		t.Fatalf("got %v, %v", names(ms), err)
	}
	spec.PathPattern = filepath.Join(dir, "report {date-1d:DD-MM-YYYY}.xlsx")
	if ms, err = Resolve(spec, slot, i18n.IT); err != nil || ms[0].Name != "report 25-10-2026.xlsx" {
		t.Fatalf("offset: got %v, %v", names(ms), err)
	}
}

func TestResolveMatchModes(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b.csv"), slot.Add(-time.Hour))
	write(t, filepath.Join(dir, "a.csv"), slot.Add(-2*time.Hour))
	write(t, filepath.Join(dir, "C.CSV"), slot.Add(-3*time.Hour))
	write(t, filepath.Join(dir, "a.txt"), slot)
	if err := os.Mkdir(filepath.Join(dir, "d.csv"), 0o700); err != nil {
		t.Fatal(err)
	}
	pattern := filepath.Join(dir, "*.csv")

	_, err := Resolve(config.FileSpec{PathPattern: pattern, Match: config.MatchError}, slot, i18n.EN)
	if code(err) != apperr.FileMultipleMatches || apperr.As(err).Params["count"] != "3" {
		t.Fatalf("error mode: %v", err)
	}
	ms, err := Resolve(config.FileSpec{PathPattern: pattern, Match: config.MatchNewest}, slot, i18n.EN)
	if err != nil || len(ms) != 1 || ms[0].Name != "b.csv" {
		t.Fatalf("newest: %v, %v", names(ms), err)
	}
	ms, err = Resolve(config.FileSpec{PathPattern: pattern, Match: config.MatchAll}, slot, i18n.EN)
	if err != nil || len(ms) != 3 || ms[0].Name != "C.CSV" || ms[1].Name != "a.csv" || ms[2].Name != "b.csv" {
		t.Fatalf("all: %v, %v", names(ms), err)
	}
	// One match is fine in error mode; ? is one character.
	ms, err = Resolve(config.FileSpec{PathPattern: filepath.Join(dir, "?.txt"), Match: config.MatchError}, slot, i18n.EN)
	if err != nil || len(ms) != 1 || ms[0].Name != "a.txt" {
		t.Fatalf("?: %v, %v", names(ms), err)
	}
}

func TestResolveNotFound(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{
		filepath.Join(dir, "missing.xlsx"),
		filepath.Join(dir, "*.xlsx"),
		filepath.Join(dir, "nodir", "*.xlsx"),
		dir + string(filepath.Separator) + "sub", // a folder is not a file
	} {
		if p == filepath.Join(dir, "sub") {
			_ = os.Mkdir(p, 0o700)
		}
		_, err := Resolve(config.FileSpec{PathPattern: p, Match: config.MatchError}, slot, i18n.EN)
		if code(err) != apperr.FileNotFound || apperr.As(err).Params["path"] != p {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestRenderValidation(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		pattern string
		want    apperr.Code
	}{
		{"", apperr.FilespecPathRequired},
		{"report.xlsx", apperr.FilespecPathNotAbsolute},
		{filepath.Join(dir, "*", "report.xlsx"), apperr.FilespecGlobInDir},
		{filepath.Join(dir, "r?p", "report.xlsx"), apperr.FilespecGlobInDir},
		{filepath.Join(dir, "{date:QQ}.xlsx"), apperr.TokenFormatInvalid},
		{filepath.Join(dir, "[1] *.xlsx"), ""},
	}
	for _, tc := range tests {
		if got := code(Validate(tc.pattern, slot, i18n.EN)); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.pattern, got, tc.want)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		p, n string
		want bool
	}{
		{"*.xlsx", "a.xlsx", true},
		{"*.xlsx", "a.XLSX", true},
		{"*.xlsx", "a.xlsx.bak", false},
		{"rep*2026*.pdf", "report 10 2026 final.pdf", true},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"[1]*", "[1] x", true},
		{"*", "", true},
		{"**a", "bba", true},
	}
	for _, tc := range tests {
		if got := globMatch(tc.p, tc.n); got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v", tc.p, tc.n, got)
		}
	}
}

func TestOpenLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held.xlsx")
	write(t, path, slot)
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	lock, err := platform.OpenExclusive(path)
	if err != nil {
		t.Skipf("no exclusive open on this platform: %v", err)
	}
	defer lock.Close()
	_, err = Open(path)
	if code(err) != apperr.FileLocked || apperr.As(err).Params["file"] != "held.xlsx" {
		t.Fatalf("got %v", err)
	}
	if code(ReadError(path, errors.New("io"))) != apperr.FileUnreadable {
		t.Fatal("a plain read error is file.unreadable")
	}
}
