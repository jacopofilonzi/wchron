package tokens

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jacopofilonzi/wchron/internal/i18n"
)

// field is one piece of a format: a letter group or literal text.
type field struct {
	code string // one of letterCodes, or "" for literal text
	text string
}

// letterCodes are the format letters, longest first within each letter so
// that the first prefix match is the longest one.
var letterCodes = []string{
	"YYYY", "YY",
	"MMMM", "MMM", "MM", "M",
	"DD", "D",
	"dddd", "ddd",
	"HH", "H",
	"mm",
	"ss",
}

// badLetter is a format letter that is not part of any code; at is its
// index in the format.
type badLetter struct {
	at     int
	letter rune
}

func (e *badLetter) Error() string { return fmt.Sprintf("unknown format letter %q", e.letter) }

func parseFormat(f []rune) ([]field, error) {
	var out []field
	var lit strings.Builder
	for i := 0; i < len(f); {
		if !isLetter(f[i]) {
			lit.WriteRune(f[i])
			i++
			continue
		}
		code := ""
		for _, c := range letterCodes {
			if hasPrefix(f[i:], c) {
				code = c
				break
			}
		}
		if code == "" {
			return nil, &badLetter{at: i, letter: f[i]}
		}
		if lit.Len() > 0 {
			out = append(out, field{text: lit.String()})
			lit.Reset()
		}
		out = append(out, field{code: code})
		i += len(code)
	}
	if lit.Len() > 0 {
		out = append(out, field{text: lit.String()})
	}
	return out, nil
}

func hasPrefix(s []rune, p string) bool {
	if len(s) < len(p) {
		return false
	}
	return string(s[:len(p)]) == p // codes are ASCII: one rune per byte
}

func (f field) write(b *strings.Builder, day civil, t time.Time, lang i18n.Lang) {
	n := names[lang]
	if n == nil {
		n = names[i18n.EN]
	}
	switch f.code {
	case "":
		b.WriteString(f.text)
	case "YYYY":
		b.WriteString(pad(day.y, 4))
	case "YY":
		b.WriteString(pad(abs(day.y)%100, 2))
	case "MMMM":
		b.WriteString(n.months[day.m-1])
	case "MMM":
		b.WriteString(n.monthsShort[day.m-1])
	case "MM":
		b.WriteString(pad(int(day.m), 2))
	case "M":
		b.WriteString(strconv.Itoa(int(day.m)))
	case "DD":
		b.WriteString(pad(day.d, 2))
	case "D":
		b.WriteString(strconv.Itoa(day.d))
	case "dddd":
		b.WriteString(n.days[day.weekday()])
	case "ddd":
		b.WriteString(n.daysShort[day.weekday()])
	case "HH":
		b.WriteString(pad(t.Hour(), 2))
	case "H":
		b.WriteString(strconv.Itoa(t.Hour()))
	case "mm":
		b.WriteString(pad(t.Minute(), 2))
	case "ss":
		b.WriteString(pad(t.Second(), 2))
	}
}

func pad(v, width int) string {
	if v < 0 {
		return "-" + pad(-v, width)
	}
	s := strconv.Itoa(v)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// nameSet holds the month and weekday names of a language (weekdays from Monday).
type nameSet struct {
	months, monthsShort [12]string
	days, daysShort     [7]string
}

// The names are data of the date format, not UI text: they live here, not
// in the locales catalog, so a missing catalog key can't break a file path.
var names = map[i18n.Lang]*nameSet{
	i18n.IT: {
		months: [12]string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno",
			"luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"},
		monthsShort: [12]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"},
		days:        [7]string{"lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato", "domenica"},
		daysShort:   [7]string{"lun", "mar", "mer", "gio", "ven", "sab", "dom"},
	},
	i18n.EN: {
		months: [12]string{"January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December"},
		monthsShort: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		days:        [7]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"},
		daysShort:   [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"},
	},
}
