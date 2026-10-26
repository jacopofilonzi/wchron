package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestMarshalJSON(t *testing.T) {
	e := New(AutostartFailed, errors.New("access denied")).With("path", `C:\x`)
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"code":"autostart.failed","params":{"path":"C:\\x"}}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	b, err = json.Marshal(New(Internal, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"code":"internal","params":{}}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestAsAndUnwrap(t *testing.T) {
	cause := errors.New("boom")
	wrapped := fmt.Errorf("context: %w", New(KeepAwakeFailed, cause))
	e := As(wrapped)
	if e.Code != KeepAwakeFailed || !errors.Is(wrapped, cause) {
		t.Fatalf("got %v", e)
	}
	if As(errors.New("plain")).Code != Internal {
		t.Fatal("an error without a code must become internal")
	}
	if string(MarshalForFrontend(errors.New("plain"))) != `{"code":"internal","params":{}}` {
		t.Fatal("MarshalForFrontend must hide the message")
	}
}

// ERRORS.md is the user-facing list: every registered code must have a row.
func TestErrorsMDListsEveryCode(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "ERRORS.md"))
	if err != nil {
		t.Fatalf("read ERRORS.md: %v", err)
	}
	row := regexp.MustCompile("(?m)^\\| `([a-z0-9_.]+)` \\|")
	listed := map[string]bool{}
	for _, m := range row.FindAllStringSubmatch(string(data), -1) {
		listed[m[1]] = true
	}
	for _, c := range All {
		if !listed[string(c)] {
			t.Errorf("code %q is registered but not listed in ERRORS.md", c)
		}
	}
}

func TestCodesAreUnique(t *testing.T) {
	seen := map[Code]bool{}
	valid := regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)*$`)
	for _, c := range All {
		if seen[c] {
			t.Errorf("duplicate code %q", c)
		}
		if !valid.MatchString(string(c)) {
			t.Errorf("code %q is not dotted lowercase", c)
		}
		seen[c] = true
	}
}
