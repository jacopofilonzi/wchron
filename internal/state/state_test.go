package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/migrate"
)

const v1Full = `{
  "schema_version": 1,
  "routine_active": false,
  "last_app_version": "0.1.0",
  "notified_update_version": "0.3.0",
  "schedules": {
    "s-1": {"last_slot": "2026-10-25T02:30:00+02:00", "last_result": "ok", "last_error": null, "last_run_at": "2026-10-25T02:30:04+02:00"},
    "s-2": {"last_result": "failed", "last_error": {"code": "file.not_found", "params": {"path": "C:\\x.pdf"}}}
  }
}`

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaults(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil || l.Exists || l.Corrupt != nil || !l.State.RoutineActive || l.State.Schedules == nil {
		t.Fatalf("%+v %v", l, err)
	}
	// A missing routine_active keeps the default (active).
	if l, err := Load(write(t, `{"schema_version": 1}`)); err != nil || !l.State.RoutineActive {
		t.Fatalf("%+v %v", l, err)
	}
}

func TestV1LoadsAndRoundTrips(t *testing.T) {
	p := write(t, v1Full)
	l, err := Load(p)
	if err != nil || l.Corrupt != nil || l.Migrated || !l.Exists {
		t.Fatalf("%+v %v", l, err)
	}
	s := l.State
	slot := time.Date(2026, 10, 25, 2, 30, 0, 0, time.FixedZone("", 2*3600))
	if s.RoutineActive || s.LastAppVersion != "0.1.0" || s.NotifiedUpdateVersion != "0.3.0" ||
		!s.Schedules["s-1"].LastSlot.Equal(slot) || s.Schedules["s-2"].LastError.Code != "file.not_found" ||
		!s.Schedules["s-2"].LastSlot.IsZero() {
		t.Fatalf("%+v", s)
	}
	if err := Save(p, s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), `"last_slot": "0001`) {
		t.Fatal("zero times must be omitted")
	}
	again, err := Load(p)
	if err != nil || again.State.Schedules["s-2"].LastError.Params["path"] != `C:\x.pdf` || again.State.RoutineActive {
		t.Fatalf("%+v %v", again, err)
	}
}

func TestCorruptGivesDefaultsAndQuarantine(t *testing.T) {
	for _, content := range []string{`{"routine_active": `, `[1]`, `{"schedules": []}`} {
		p := write(t, content)
		l, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if e := apperr.As(l.Corrupt); l.Corrupt == nil || e.Code != apperr.StateReadFailed || e.Params["path"] != p {
			t.Fatalf("%s: corrupt %v", content, l.Corrupt)
		}
		if !l.State.RoutineActive || l.State.Schedules == nil {
			t.Fatalf("%s: not the defaults: %+v", content, l.State)
		}
		dst, err := Quarantine(p, time.Date(2026, 10, 26, 9, 5, 7, 0, time.Local))
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(dst) != "state.json.corrupt-20261026-090507" {
			t.Fatalf("dst %s", dst)
		}
		if b, err := os.ReadFile(dst); err != nil || string(b) != content {
			t.Fatalf("the bad file must be kept as is: %q %v", b, err)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("state.json must be gone")
		}
	}
}

func TestNewerIsFatalAndUntouched(t *testing.T) {
	p := write(t, `{"schema_version": 7}`)
	_, err := Load(p)
	if e := apperr.As(err); e.Code != apperr.ConfigNewerVersion || e.Params["found"] != "7" {
		t.Fatalf("err %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != `{"schema_version": 7}` {
		t.Fatal("file changed")
	}
}

func TestMigrationWithTestSteps(t *testing.T) {
	// A fake v1→v2 step that renames "paused" to "routine_active" (inverted).
	testSteps := []migrate.Step{func(doc map[string]any) error {
		if v, ok := doc["paused"].(bool); ok {
			doc["routine_active"] = !v
			delete(doc, "paused")
		}
		return nil
	}}
	l, err := load(write(t, `{"schema_version": 1, "paused": true}`), testSteps)
	if err != nil || !l.Migrated || l.From != 1 || l.State.RoutineActive {
		t.Fatalf("%+v %v", l, err)
	}
	failing := []migrate.Step{func(map[string]any) error { return os.ErrInvalid }}
	if _, err := load(write(t, `{}`), failing); apperr.As(err).Code != apperr.ConfigMigrationFailed {
		t.Fatalf("err %v", err)
	}
}

func TestSaveFailure(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope", "state.json")
	if e := apperr.As(Save(p, Default())); e.Code != apperr.StateWriteFailed || e.Params["path"] != p {
		t.Fatalf("err %v", e)
	}
}
