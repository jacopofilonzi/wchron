package i18n

import (
	"sort"
	"testing"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

func TestCatalogsHaveTheSameKeys(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ref := cat[EN]
	if len(ref) == 0 {
		t.Fatal("en.json is empty")
	}
	for _, l := range Langs {
		var missing, extra []string
		for k := range ref {
			if _, ok := cat[l][k]; !ok {
				missing = append(missing, k)
			}
		}
		for k, v := range cat[l] {
			if _, ok := ref[k]; !ok {
				extra = append(extra, k)
			}
			if v == "" {
				t.Errorf("%s.json: %q is empty", l, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		if len(missing) > 0 {
			t.Errorf("%s.json is missing %v", l, missing)
		}
		if len(extra) > 0 {
			t.Errorf("%s.json has keys not in en.json: %v", l, extra)
		}
	}
}

func TestEveryErrorCodeHasAText(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range apperr.All {
		for _, l := range Langs {
			if _, ok := cat[l]["error."+string(c)]; !ok {
				t.Errorf("%s.json has no text for error.%s", l, c)
			}
		}
	}
}

func TestT(t *testing.T) {
	if got := T(EN, "tray.status.channels", map[string]string{"state": "x"}); got != "Channels: x" {
		t.Fatalf("got %q", got)
	}
	if got := T(IT, "tray.quit", nil); got != "Esci" {
		t.Fatalf("got %q", got)
	}
	if got := T(Lang("fr"), "tray.quit", nil); got != "Quit" {
		t.Fatalf("unknown language must fall back to English, got %q", got)
	}
	if got := T(EN, "no.such.key", nil); got != "no.such.key" {
		t.Fatalf("got %q", got)
	}
	if got := T(EN, "tray.status.channels", map[string]string{"other": "x"}); got != "Channels: {state}" {
		t.Fatalf("a missing param must stay visible, got %q", got)
	}
}
