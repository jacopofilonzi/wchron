package tokens

import (
	"errors"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Rome for the DST cases on any machine (tests only)

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
)

// Monday 26 October 2026, 09:05:07, one hour east of UTC.
var ref = time.Date(2026, 10, 26, 9, 5, 7, 0, time.FixedZone("CET", 3600))

func render(t *testing.T, tpl string, at time.Time, lang i18n.Lang) string {
	t.Helper()
	out, err := Render(tpl, at, lang)
	if err != nil {
		t.Fatalf("Render(%q): %v", tpl, err)
	}
	return out
}

func TestFormatLetters(t *testing.T) {
	cases := []struct{ format, it, en string }{
		{"YYYY", "2026", "2026"},
		{"YY", "26", "26"},
		{"MMMM", "ottobre", "October"},
		{"MMM", "ott", "Oct"},
		{"MM", "10", "10"},
		{"M", "10", "10"},
		{"DD", "26", "26"},
		{"D", "26", "26"},
		{"dddd", "lunedì", "Monday"},
		{"ddd", "lun", "Mon"},
		{"HH", "09", "09"},
		{"H", "9", "9"},
		{"mm", "05", "05"},
		{"ss", "07", "07"},
		// longest match first, adjacent codes, literals
		{"MMMMM", "ottobre10", "October10"},
		{"DDMMYYYY", "26102026", "26102026"},
		{"dddd DD MMMM", "lunedì 26 ottobre", "Monday 26 October"},
		{"HH.mm_ss -/ 1", "09.05_07 -/ 1", "09.05_07 -/ 1"},
		{"HH:mm", "09:05", "09:05"},
		{"DD à-è ü", "26 à-è ü", "26 à-è ü"}, // accented letters are literal
	}
	for _, c := range cases {
		if got := render(t, "{date:"+c.format+"}", ref, i18n.IT); got != c.it {
			t.Errorf("it %q: got %q, want %q", c.format, got, c.it)
		}
		if got := render(t, "{date:"+c.format+"}", ref, i18n.EN); got != c.en {
			t.Errorf("en %q: got %q, want %q", c.format, got, c.en)
		}
	}
}

func TestPadding(t *testing.T) {
	at := time.Date(2005, 3, 4, 0, 0, 0, 0, time.UTC)
	if got := render(t, "{date:YYYY YY MM M DD D HH H mm ss}", at, i18n.IT); got != "2005 05 03 3 04 4 00 0 00 00" {
		t.Fatalf("got %q", got)
	}
}

func TestAllNames(t *testing.T) {
	itDays := []string{"lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato", "domenica"}
	enDays := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
	itShort := []string{"lun", "mar", "mer", "gio", "ven", "sab", "dom"}
	for i := range 7 {
		day := time.Date(2026, 10, 26+i, 12, 0, 0, 0, time.UTC) // Monday 26 → Sunday 1 November
		if got := render(t, "{date:dddd ddd}", day, i18n.IT); got != itDays[i]+" "+itShort[i] {
			t.Errorf("it day %d: got %q", i, got)
		}
		if got := render(t, "{date:dddd}", day, i18n.EN); got != enDays[i] {
			t.Errorf("en day %d: got %q", i, got)
		}
	}
	itMonths := []string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno",
		"luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}
	itShortM := []string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"}
	for m := range 12 {
		at := time.Date(2026, time.Month(m+1), 15, 12, 0, 0, 0, time.UTC)
		if got := render(t, "{date:MMMM MMM}", at, i18n.IT); got != itMonths[m]+" "+itShortM[m] {
			t.Errorf("it month %d: got %q", m+1, got)
		}
	}
	if got := render(t, "{date:MMM MMMM}", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), i18n.EN); got != "May May" {
		t.Errorf("en May: got %q", got)
	}
	if got := render(t, "{date:MMMM}", ref, i18n.Lang("fr")); got != "October" {
		t.Errorf("unknown language must render in English, got %q", got)
	}
}

func TestModifiers(t *testing.T) {
	cases := []struct {
		tpl  string
		at   time.Time
		want string
	}{
		{"{date}", ref, "2026-10-26"}, // default format
		{"{date-1d}", ref, "2026-10-25"},
		{"{date+7d}", ref, "2026-11-02"},
		{"{date-2w}", ref, "2026-10-12"},
		{"{date+1m}", ref, "2026-11-26"},
		{"{date-10m}", ref, "2025-12-26"},
		{"{date+1y}", ref, "2027-10-26"},
		{"{date-1d-1d+0d}", ref, "2026-10-24"},
		// month and year offsets clamp the day
		{"{date+1m}", day(2026, 1, 31), "2026-02-28"},
		{"{date+1m}", day(2028, 1, 31), "2028-02-29"}, // leap year
		{"{date-1m}", day(2026, 3, 31), "2026-02-28"},
		{"{date+1m+1m}", day(2026, 1, 31), "2026-03-28"}, // left to right: the clamp sticks
		{"{date+2m}", day(2026, 1, 31), "2026-03-31"},
		{"{date+1y}", day(2028, 2, 29), "2029-02-28"},
		{"{date-4y}", day(2028, 2, 29), "2024-02-29"},
		{"{date+1d}", day(2028, 2, 28), "2028-02-29"},
		{"{date+1d}", day(2026, 12, 31), "2027-01-01"},
		// anchors
		{"{date@month-start}", ref, "2026-10-01"},
		{"{date@month-end}", ref, "2026-10-31"},
		{"{date@month-end}", day(2028, 2, 10), "2028-02-29"},
		{"{date@year-start}", ref, "2026-01-01"},
		{"{date@year-end}", ref, "2026-12-31"},
		{"{date@week-start}", ref, "2026-10-26"}, // ref is a Monday
		{"{date@week-end}", ref, "2026-11-01"},
		{"{date@week-start}", day(2026, 11, 1), "2026-10-26"}, // Sunday
		{"{date@week-end}", day(2026, 11, 1), "2026-11-01"},
		{"{date@week-start}", day(2026, 1, 1), "2025-12-29"}, // across the year
		// chained, both orders
		{"{date@month-start-1m:MMMM}", ref, "settembre"},
		{"{date-1m@month-start}", day(2026, 3, 31), "2026-02-01"},
		{"{date@month-start-1d}", ref, "2026-09-30"},
		{"{date-1d@month-start}", ref, "2026-10-01"},
		{"{date-1d@week-start:DD/MM}", ref, "19/10"},
		{"{date@week-start-1d:DD/MM}", ref, "25/10"},
		{"{date@month-end@week-end}", ref, "2026-11-01"},
		// anchors and offsets keep the time of day
		{"{date@month-start-1m:YYYY-MM-DD HH:mm:ss}", ref, "2026-09-01 09:05:07"},
	}
	for _, c := range cases {
		if got := render(t, c.tpl, c.at, i18n.IT); got != c.want {
			t.Errorf("%s at %s: got %q, want %q", c.tpl, c.at.Format("2006-01-02"), got, c.want)
		}
	}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 10, 0, 0, 0, time.UTC) }

func TestLiteralsAndEscaping(t *testing.T) {
	cases := []struct{ tpl, want string }{
		{"", ""},
		{"report.xlsx", "report.xlsx"},
		{`C:\Report città\vendite_{date:DD_MM_YYYY}.xlsx`, `C:\Report città\vendite_26_10_2026.xlsx`},
		{"{{date}}", "{date}"},
		{"{{{date}}}", "{2026-10-26}"},
		{"a {{ b }} c", "a { b } c"},
		{"è{date:D}ü", "è26ü"},
		{"{date:YY}{date:MM}", "2610"},
	}
	for _, c := range cases {
		if got := render(t, c.tpl, ref, i18n.IT); got != c.want {
			t.Errorf("%q: got %q, want %q", c.tpl, got, c.want)
		}
	}
}

func TestHasTokens(t *testing.T) {
	for tpl, want := range map[string]bool{"": false, "a.xlsx": false, "{{date}}": false, "a{date}": true} {
		p, err := Parse(tpl)
		if err != nil {
			t.Fatal(err)
		}
		if p.HasTokens() != want {
			t.Errorf("%q: HasTokens = %v", tpl, !want)
		}
	}
}

func TestParseOnceRenderMany(t *testing.T) {
	p, err := Parse("{date-1d:DD}")
	if err != nil {
		t.Fatal(err)
	}
	if a, b := p.Render(day(2026, 1, 10), i18n.IT), p.Render(day(2026, 3, 1), i18n.EN); a != "09" || b != "28" {
		t.Fatalf("got %q, %q", a, b)
	}
}

// The date comes from the calendar in the time's own location: adding hours
// would land on the wrong day around the DST changes (Europe/Rome, 2026:
// 29 March has 23 hours, 25 October 25).
func TestDST(t *testing.T) {
	rome, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatal(err)
	}
	at := func(m time.Month, d, h, mi int) time.Time { return time.Date(2026, m, d, h, mi, 0, 0, rome) }
	cases := []struct {
		tpl  string
		at   time.Time
		want string
	}{
		{"{date-1d:YYYY-MM-DD HH:mm}", at(3, 30, 0, 30), "2026-03-29 00:30"},
		{"{date+1d:YYYY-MM-DD HH:mm}", at(3, 28, 23, 30), "2026-03-29 23:30"},
		{"{date:YYYY-MM-DD HH:mm}", at(3, 29, 3, 0), "2026-03-29 03:00"},
		{"{date-1w}", at(4, 5, 0, 15), "2026-03-29"},
		{"{date-1d:YYYY-MM-DD HH:mm}", at(10, 26, 0, 30), "2026-10-25 00:30"},
		{"{date+1d:YYYY-MM-DD HH:mm}", at(10, 24, 23, 30), "2026-10-25 23:30"},
		{"{date:YYYY-MM-DD HH:mm}", at(10, 25, 23, 59), "2026-10-25 23:59"},
		{"{date@week-start}", at(10, 25, 23, 59), "2026-10-19"},
		// the same instant in UTC is on another calendar day: the location counts
		{"{date:DD}", at(3, 30, 0, 30), "30"},
		{"{date:DD}", at(3, 30, 0, 30).UTC(), "29"},
	}
	for _, c := range cases {
		if got := render(t, c.tpl, c.at, i18n.IT); got != c.want {
			t.Errorf("%s at %s: got %q, want %q", c.tpl, c.at, got, c.want)
		}
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		tpl    string
		code   apperr.Code
		pos    string
		param  string // name of the extra param, "" for none
		pvalue string
	}{
		{"{date", apperr.TokenUnclosed, "1", "", ""},
		{"abc{date:DD", apperr.TokenUnclosed, "4", "", ""},
		{"{date{date}", apperr.TokenUnclosed, "1", "", ""},
		{"{", apperr.TokenUnclosed, "1", "", ""},
		{"àè{", apperr.TokenUnclosed, "3", "", ""}, // runes, not bytes
		{"}", apperr.TokenUnexpectedClose, "1", "", ""},
		{"{date}}", apperr.TokenUnexpectedClose, "7", "", ""},
		{"città}", apperr.TokenUnexpectedClose, "6", "", ""},
		{"x}}}", apperr.TokenUnexpectedClose, "4", "", ""},
		{"{}", apperr.TokenEmpty, "1", "", ""},
		{"ab{  }", apperr.TokenEmpty, "3", "", ""},
		{"{data}", apperr.TokenUnknown, "2", "name", "data"},
		{"{Date}", apperr.TokenUnknown, "2", "name", "Date"},
		{"é{date x}", apperr.TokenUnknown, "3", "name", "date x"},
		{"{:YYYY}", apperr.TokenUnknown, "2", "name", ""},
		{"{-1d}", apperr.TokenUnknown, "2", "name", ""},
		{"{date:}", apperr.TokenFormatInvalid, "6", "format", ""},
		{"{date:YYYY-MM-Q}", apperr.TokenFormatInvalid, "15", "format", "YYYY-MM-Q"},
		{"{date:YYY}", apperr.TokenFormatInvalid, "9", "format", "YYY"},
		{"{date:hh}", apperr.TokenFormatInvalid, "7", "format", "hh"},
		{"{date:d}", apperr.TokenFormatInvalid, "7", "format", "d"},
		{"{date:dd}", apperr.TokenFormatInvalid, "7", "format", "dd"},
		{"{date:ddddd}", apperr.TokenFormatInvalid, "11", "format", "ddddd"},
		{"ù{date:DD è X}", apperr.TokenFormatInvalid, "13", "format", "DD è X"},
		{"{date-1}", apperr.TokenOffsetInvalid, "6", "offset", "-1"},
		{"{date-d}", apperr.TokenOffsetInvalid, "6", "offset", "-d"},
		{"{date+1x}", apperr.TokenOffsetInvalid, "6", "offset", "+1x"},
		{"{date-1dd}", apperr.TokenOffsetInvalid, "6", "offset", "-1dd"},
		{"{date-1D}", apperr.TokenOffsetInvalid, "6", "offset", "-1D"},
		{"{date-12345d}", apperr.TokenOffsetInvalid, "6", "offset", "-12345d"},
		{"{date+ 1d}", apperr.TokenOffsetInvalid, "6", "offset", "+ 1d"},
		{"{date-1d-}", apperr.TokenOffsetInvalid, "9", "offset", "-"},
		{"{date@}", apperr.TokenOffsetInvalid, "6", "offset", "@"},
		{"{date@month}", apperr.TokenOffsetInvalid, "6", "offset", "@month"},
		{"{date@month-xyz-1d}", apperr.TokenOffsetInvalid, "6", "offset", "@month-xyz"},
		{"{date@month-starts}", apperr.TokenOffsetInvalid, "6", "offset", "@month-starts"},
		{"{date@Month-start}", apperr.TokenOffsetInvalid, "6", "offset", "@Month-start"},
		{"àà{date-1d@nope:DD}", apperr.TokenOffsetInvalid, "11", "offset", "@nope"},
		// the first error wins
		{"{data} }", apperr.TokenUnknown, "2", "name", "data"},
		{"} {data}", apperr.TokenUnexpectedClose, "1", "", ""},
	}
	for _, c := range cases {
		_, err := Parse(c.tpl)
		var e *apperr.Error
		if !errors.As(err, &e) {
			t.Errorf("%q: got %v, want %s", c.tpl, err, c.code)
			continue
		}
		if e.Code != c.code || e.Params["pos"] != c.pos {
			t.Errorf("%q: got %s pos %s, want %s pos %s", c.tpl, e.Code, e.Params["pos"], c.code, c.pos)
		}
		want := 1
		if c.param != "" {
			want = 2
			if v, ok := e.Params[c.param]; !ok || v != c.pvalue {
				t.Errorf("%q: %s = %q (set %v), want %q", c.tpl, c.param, v, ok, c.pvalue)
			}
		}
		if len(e.Params) != want {
			t.Errorf("%q: params %v", c.tpl, e.Params)
		}
		if _, err := Render(c.tpl, ref, i18n.IT); err == nil {
			t.Errorf("%q: Render must fail too", c.tpl)
		}
	}
}

// Malformed input never panics: every one of these returns an error or a result.
func TestMalformedNeverPanics(t *testing.T) {
	inputs := []string{
		"{", "}", "{{", "}}", "{{{", "}}}", "{}{}", "{:", "{:}", "{@", "{@}", "{+}", "{-}", "{date@",
		"{date+", "{date-9999y}", "{date+9999y@year-end:YYYY}", "{date:\x00}", "\xff\xfe{date}",
		"{date@week-start@week-start@week-start}", "{date::::}", "{date-}", "{date@-1d}", "{date@month-}",
		"{date:" + string(rune(0x10FFFF)) + "}", "{date-0000d}", "{date+1m-1m+1y-1y@month-end-1d}",
	}
	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%q: panic %v", in, r)
				}
			}()
			_, _ = Render(in, ref, i18n.IT)
		}()
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"{date}", "{date-1d@week-start:DD/MM}", "{{x}}", "a}b{", "{date@month-xyz-1d}", "è{date:MMMM}"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		tpl, err := Parse(s)
		if err != nil {
			var e *apperr.Error
			if !errors.As(err, &e) || e.Params["pos"] == "" {
				t.Fatalf("%q: error without code or pos: %v", s, err)
			}
			return
		}
		_ = tpl.Render(ref, i18n.IT)
	})
}
