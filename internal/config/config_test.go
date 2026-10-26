package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/migrate"
)

// v1Full is a realistic version 1 file (synthetic data).
const v1Full = `{
  "schema_version": 1,
  "settings": {
    "error_recipients": [{"id": "r-err", "channel_id": "wa-1", "target": {"kind": "number", "number": "+390000000001"}, "display_name": "Me"}],
    "keep_awake_default": true,
    "language": "en",
    "theme": "dark"
  },
  "channels": [{"id": "wa-1", "provider": "whatsapp", "name": "WhatsApp personale", "enabled": true, "settings": {"big_id": 12345678901234567890}}],
  "schedules": [{
    "id": "s-1", "name": "Daily report", "enabled": true,
    "cadence": {"mode": "interval", "interval": {"hours": 1, "minutes": 30}, "anchor": "08:00",
                "window": {"start": "08:00", "end": "18:00"}, "times": [], "days": ["mon", "tue", "wed", "thu", "fri"]},
    "recipients": [{"id": "r-1", "channel_id": "wa-1", "target": {"kind": "group", "jid": "000@g.us"}, "display_name": "Team", "verified": "yes"}],
    "caption": "Report {date:DD-MM-YYYY}",
    "files": [{"id": "f-1", "path_pattern": "C:\\Reports\\{date:YYYY-MM-DD}.xlsx", "match": "newest",
               "processor": "xlsx-sheets", "processor_options": {"sheets": ["Summary"]}, "caption": "Only this"}]
  }]
}`

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFreshInstallDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	l, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	c := l.Config
	if l.Exists || l.Migrated || c.SchemaVersion != CurrentVersion || c.Settings.Language != i18n.IT ||
		c.Settings.Theme != ThemeSystem || c.Settings.KeepAwakeDefault || c.Channels == nil || c.Schedules == nil ||
		c.Settings.ErrorRecipients == nil {
		t.Fatalf("defaults: %+v", l)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("Load must not write the file")
	}
}

func TestV1LoadsUnchangedAndRoundTrips(t *testing.T) {
	p := write(t, v1Full)
	l, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Exists || l.Migrated || l.From != 1 {
		t.Fatalf("load: %+v", l)
	}
	c := l.Config
	s := c.Schedules[0]
	if c.Settings.Language != i18n.EN || c.Settings.Theme != ThemeDark || !c.Settings.KeepAwakeDefault ||
		s.Cadence.Window == nil || s.Cadence.Window.End != "18:00" || len(s.Cadence.Days) != 5 ||
		s.Files[0].Match != MatchNewest || s.Files[0].Caption == nil || *s.Files[0].Caption != "Only this" ||
		string(s.Recipients[0].Target) != `{"jid":"000@g.us","kind":"group"}` || s.Recipients[0].Verified != VerifiedYes ||
		c.Settings.ErrorRecipients[0].Verified != VerifiedUnknown {
		t.Fatalf("decoded: %+v", c)
	}
	if string(c.Channels[0].Settings) != `{"big_id":12345678901234567890}` {
		t.Fatalf("numbers must stay exact: %s", c.Channels[0].Settings)
	}
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	again, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(c)
	b, _ := json.Marshal(again.Config)
	if string(a) != string(b) {
		t.Fatalf("round trip differs:\n%s\n%s", a, b)
	}
}

func TestMissingFieldsAreNormalized(t *testing.T) {
	p := write(t, `{"schema_version": 1, "settings": {"theme": "light"},
	  "channels": [{"id": "c", "provider": "whatsapp"}],
	  "schedules": [{"id": "s", "cadence": {"mode": "fixed", "times": ["09:00"]}, "files": [{"id": "f", "path_pattern": "C:\\a.pdf"}]}]}`)
	l, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	c := l.Config
	s := c.Schedules[0]
	if c.Settings.Language != i18n.IT || c.Settings.Theme != ThemeLight || c.Settings.ErrorRecipients == nil ||
		string(c.Channels[0].Settings) != "{}" || s.Recipients == nil || s.Cadence.Days == nil ||
		s.Files[0].Match != MatchError || s.Files[0].Processor != ProcessorPassthrough ||
		string(s.Files[0].ProcessorOptions) != "{}" || s.Files[0].Caption != nil {
		t.Fatalf("normalized: %+v", c)
	}
	if l, err := Load(write(t, `{}`)); err != nil || l.From != 1 || l.Config.Settings.Theme != ThemeSystem {
		t.Fatalf("empty object: %+v %v", l, err)
	}
}

func TestNewerVersionRefusedAndUntouched(t *testing.T) {
	content := `{"schema_version": 99, "settings": {"language": "en"}}`
	p := write(t, content)
	_, err := Load(p)
	e := apperr.As(err)
	if e.Code != apperr.ConfigNewerVersion || e.Params["found"] != "99" || e.Params["supported"] != "1" {
		t.Fatalf("err %v %v", e, e.Params)
	}
	assertContent(t, p, content)
}

func TestUnparseableRefusedAndUntouched(t *testing.T) {
	for _, content := range []string{`{"schema_version": 1, "settings": `, `[]`, `{"settings": {"keep_awake_default": "yes"}}`} {
		p := write(t, content)
		_, err := Load(p)
		if e := apperr.As(err); e.Code != apperr.ConfigReadFailed || e.Params["path"] != p {
			t.Fatalf("%s: err %v", content, err)
		}
		assertContent(t, p, content)
	}
}

func TestInvalidStructure(t *testing.T) {
	cases := map[string]string{
		`{"settings": {"theme": "blue"}}`:                                                                  "settings.theme",
		`{"settings": {"language": "fr"}}`:                                                                 "settings.language",
		`{"channels": [{"id": "a", "provider": "x"}, {"id": "a", "provider": "x"}]}`:                       "channels[1].id",
		`{"channels": [{"id": "a"}]}`:                                                                      "channels[0].provider",
		`{"schedules": [{"id": "s", "cadence": {"mode": "x"}}]}`:                                           "schedules[0].cadence.mode",
		`{"schedules": [{"id": "s", "cadence": {"mode": "fixed", "days": ["lun"]}}]}`:                      "schedules[0].cadence.days[0]",
		`{"schedules": [{"id": "s", "cadence": {"mode": "fixed"}, "files": [{"id": "f", "match": "x"}]}]}`: "schedules[0].files[0].match",
		`{"schedules": [{"id": "s", "cadence": {"mode": "fixed"}, "recipients": [{"id": "r"}]}]}`:          "schedules[0].recipients[0].channel_id",
		`{"schedules": [{"cadence": {"mode": "fixed"}}]}`:                                                  "schedules[0].id",
		`{"settings": {"error_recipients": [{"id": "r", "channel_id": "c", "verified": "maybe"}]}}`:        "settings.error_recipients[0].verified",
	}
	for content, field := range cases {
		p := write(t, content)
		_, err := Load(p)
		if e := apperr.As(err); e.Code != apperr.ConfigInvalid || e.Params["field"] != field || e.Params["path"] != p {
			t.Errorf("%s: err %v %v", content, err, e.Params)
		}
		assertContent(t, p, content)
	}
}

func TestSaveRefusesInvalidAndFailsWithCode(t *testing.T) {
	c := Default()
	c.Settings.Theme = "blue"
	p := filepath.Join(t.TempDir(), "config.json")
	if e := apperr.As(Save(p, c)); e.Code != apperr.ConfigInvalid {
		t.Fatalf("err %v", e)
	}
	missing := filepath.Join(t.TempDir(), "nope", "config.json")
	if e := apperr.As(Save(missing, Default())); e.Code != apperr.ConfigWriteFailed || e.Params["path"] != missing {
		t.Fatalf("err %v", e)
	}
}

func TestSaveFailureKeepsOldFile(t *testing.T) {
	p := write(t, v1Full)
	// A read-only target can't be replaced on Windows: the rename fails.
	if err := os.Chmod(p, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o600) })
	c := Default()
	if err := Save(p, c); err == nil {
		// Some systems allow replacing a read-only file: then the content is new and complete.
		l, lerr := Load(p)
		if lerr != nil || l.Config.Settings.Theme != ThemeSystem {
			t.Fatalf("replaced file is not valid: %v", lerr)
		}
		return
	}
	assertContent(t, p, v1Full)
}

func TestMigrationWithTestSteps(t *testing.T) {
	// A fake v1→v2 step that renames settings.theme_name to settings.theme.
	testSteps := []migrate.Step{func(doc map[string]any) error {
		s, ok := doc["settings"].(map[string]any)
		if !ok {
			return nil
		}
		if v, ok := s["theme_name"]; ok {
			s["theme"] = v
			delete(s, "theme_name")
		}
		return nil
	}}
	p := write(t, `{"schema_version": 1, "settings": {"theme_name": "dark"}}`)
	l, err := load(p, testSteps)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Migrated || l.From != 1 || l.Config.Settings.Theme != ThemeDark {
		t.Fatalf("migrated: %+v", l)
	}
	failing := []migrate.Step{func(map[string]any) error { return os.ErrInvalid }}
	_, err = load(p, failing)
	if e := apperr.As(err); e.Code != apperr.ConfigMigrationFailed || e.Params["from"] != "1" || e.Params["to"] != "2" {
		t.Fatalf("err %v", err)
	}
}

func TestPeekLanguage(t *testing.T) {
	if PeekLanguage(write(t, `{"settings": {"language": "en"}, "schedules": "broken"}`)) != i18n.EN {
		t.Fatal("readable language")
	}
	if PeekLanguage(write(t, `{`)) != i18n.IT || PeekLanguage(filepath.Join(t.TempDir(), "x")) != i18n.IT {
		t.Fatal("fallback")
	}
}

func assertContent(t *testing.T, p, want string) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != want {
		t.Fatalf("file changed: %s", b)
	}
}
