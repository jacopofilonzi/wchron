package resolver

import (
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
)

func TestSendName(t *testing.T) {
	at := time.Date(2026, 10, 26, 9, 5, 0, 0, time.UTC)
	for _, c := range []struct{ tpl, out, want string }{
		{"", "report.xlsx", "report.xlsx"},
		{"Vendite {date:DD-MM-YYYY}", "report.xlsx", "Vendite 26-10-2026.xlsx"},
		{"Vendite {date:DD.MM.YYYY}", "report.xlsx", "Vendite 26.10.2026.xlsx"},
		{"Vendite.xlsx", "report.xlsx", "Vendite.xlsx"},
		{"data.csv", "data.txt", "data.csv"},
		{"v1.2 release", "a.pdf", "v1.2 release.pdf"},
		{"Report", "noext", "Report"},
		{"{date:MMMM}", "a.pdf", "ottobre.pdf"},
	} {
		got, err := SendName(c.tpl, c.out, at, i18n.IT)
		if err != nil || got != c.want {
			t.Errorf("SendName(%q, %q) = %q, %v; want %q", c.tpl, c.out, got, err, c.want)
		}
	}
	for _, tpl := range []string{"a:b", "a/b", `a\b`, "a*", "a?", `a"b`, "a<b", "a>b", "a|b", "a\tb", "report.", "report ",
		"CON", "con.txt", "Lpt1", "nul .pdf", "   ", string(make([]rune, 300))} {
		if _, err := SendName(tpl, "x", at, i18n.IT); apperr.As(err).Code != apperr.FilespecSendNameInvalid {
			t.Errorf("SendName(%q): %v, want send_name_invalid", tpl, err)
		}
		if err := ValidateSendName(tpl, at, i18n.IT); apperr.As(err).Code != apperr.FilespecSendNameInvalid {
			t.Errorf("ValidateSendName(%q): %v", tpl, err)
		}
	}
	// A name valid by itself whose extension is appended stays valid.
	if _, err := SendName("CONSOLE", "a.pdf", at, i18n.IT); err != nil {
		t.Fatal(err)
	}
	// A token that renders to an invalid name ("/" from the format) at run time.
	if _, err := SendName("r {date:DD/MM}", "a.pdf", at, i18n.IT); apperr.As(err).Code != apperr.FilespecSendNameInvalid ||
		apperr.As(err).Params["value"] != "r 26/10.pdf" {
		t.Fatalf("rendered slash: %v", err)
	}
	if err := ValidateSendName("{date:QQ}", at, i18n.IT); apperr.As(err).Code != apperr.TokenFormatInvalid {
		t.Fatalf("bad token: %v", err)
	}
	if err := ValidateSendName("", at, i18n.IT); err != nil {
		t.Fatal(err)
	}
}

func TestSendNameNumbered(t *testing.T) {
	at := time.Date(2026, 10, 26, 9, 5, 0, 0, time.UTC)
	for _, c := range []struct {
		tpl, out string
		n, total int
		want     string
	}{
		{"Report", "a.xlsx", 1, 2, "Report (1).xlsx"},
		{"Report", "b.xlsx", 2, 2, "Report (2).xlsx"},
		{"Report", "a.xlsx", 1, 1, "Report.xlsx"},
		{"", "a.xlsx", 1, 3, "a.xlsx"},
		{"Report {date:DD.MM.YYYY}", "a.pdf", 3, 3, "Report 26.10.2026 (3).pdf"},
		{"data.csv", "x.txt", 2, 2, "data (2).csv"},
		{"Report", "noext", 2, 2, "Report (2)"},
	} {
		got, err := SendNameNumbered(c.tpl, c.out, at, i18n.IT, c.n, c.total)
		if err != nil || got != c.want {
			t.Errorf("SendNameNumbered(%q, %q, %d/%d) = %q, %v; want %q", c.tpl, c.out, c.n, c.total, got, err, c.want)
		}
	}
	if _, err := SendNameNumbered("a/b", "x.pdf", at, i18n.IT, 1, 2); apperr.As(err).Code != apperr.FilespecSendNameInvalid {
		t.Fatalf("invalid: %v", err)
	}
}
