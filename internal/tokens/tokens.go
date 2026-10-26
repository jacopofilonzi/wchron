// Package tokens is the date token engine shared by file path patterns,
// captions and Excel sheet names.
//
// # Syntax
//
// A template is literal text with tokens in braces. Literal text is copied
// as is (accents and any other character included); `{{` writes a literal
// `{` and `}}` a literal `}`.
//
// A token is `{date<modifiers>[:<format>]}`. `date` is the only token name
// today; names live in a table (see kinds) so more can be added without
// touching the parser.
//
// Modifiers follow the name, any number, applied left to right:
//
//   - offset `+N` / `-N` followed by a unit: `d` days, `w` weeks, `m` months,
//     `y` years (`-1d`, `+2w`, `-1m`). N is 1 to 4 digits. Month and year
//     offsets clamp the day to the last day of the target month
//     (31 January +1m is 28 or 29 February; 29 February +1y is 28 February).
//   - anchor `@month-start`, `@month-end`, `@week-start` (Monday),
//     `@week-end` (Sunday), `@year-start`, `@year-end`.
//
// So `{date@month-start-1m:MMMM}` is the name of last month and
// `{date-1d@week-start:DD/MM}` the Monday of yesterday's week.
//
// The format comes after the first `:` (so `{date:HH:mm}` works); without
// one it is `YYYY-MM-DD`. Letters, longest match first: YYYY (2026), YY (26),
// MMMM (month name), MMM (short month name), MM (01-12), M (1-12), DD
// (01-31), D (1-31), dddd (weekday name), ddd (short weekday name), HH
// (00-23), H (0-23), mm (00-59), ss (00-59). Any other ASCII letter is
// token.format_invalid; every other character (`-` `_` `.` `/` space, digits,
// accented letters…) is literal. Names follow the language: Italian lowercase
// as Italian writes them (lunedì, lun, gennaio, gen), English capitalized
// (Monday, Mon, January, Jan); an unknown language renders in English.
// `MMMMM` is MMMM followed by M, `YYY` is YY followed by an invalid Y.
//
// # Errors
//
// Parse reports the first error only, as an apperr.Error whose `pos` param is
// the 1-based character (rune, not byte) position in the template, for the UI:
//
//   - token.unclosed (pos of the `{`): no `}` before the end of the template
//     or before another `{`.
//   - token.unexpected_close (pos of the `}`): a `}` outside a token, not doubled.
//   - token.empty (pos of the `{`): `{}` or only spaces inside.
//   - token.unknown (+name; pos of the name's first character, or of what
//     follows `{` when the name is empty): the name before the first
//     modifier or `:` is not a known token (`{data}`, `{Date}`, `{:YYYY}`).
//   - token.offset_invalid (+offset, the modifier as written; pos of its
//     `+`, `-` or `@`): bad number, unit or anchor, or trailing garbage.
//   - token.format_invalid (+format, the whole format; pos of the first
//     invalid letter, or of the `:` when the format is empty).
//
// # Rendering
//
// Parse once, Render many. Render uses the time it is given (the pipeline
// passes the slot time, the preview the clock's now), in that time's own
// location: the app never loads a named zone, so it is the system one.
// Offsets and anchors work on the calendar date in that location (computed
// on a plain year/month/day, never by adding hours), so a DST change can't
// shift the date; HH, mm and ss always come from the given time, anchors and
// offsets keep the time of day as it is.
package tokens

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
)

// DefaultFormat is the format of a token written without `:`.
const DefaultFormat = "YYYY-MM-DD"

// Template is a parsed template, ready to render any number of times.
type Template struct {
	parts []part
}

// part is either literal text (tok nil) or a token.
type part struct {
	text string
	tok  *token
}

type token struct {
	mods   []modifier
	format []field
}

// kinds lists the known token names. Every name is a date token today; a
// future name (e.g. a counter) gets its own entry and render branch.
var kinds = map[string]bool{"date": true}

// Parse validates template and returns it parsed, or the first error found.
func Parse(template string) (*Template, error) {
	rs := []rune(template)
	tpl := &Template{}
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			tpl.parts = append(tpl.parts, part{text: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '{':
			if i+1 < len(rs) && rs[i+1] == '{' {
				lit.WriteRune('{')
				i++
				continue
			}
			end := -1
			for j := i + 1; j < len(rs); j++ {
				if rs[j] == '}' {
					end = j
					break
				}
				if rs[j] == '{' {
					break
				}
			}
			if end < 0 {
				return nil, posErr(apperr.TokenUnclosed, i, "unclosed {")
			}
			tok, err := parseToken(rs[i+1:end], i)
			if err != nil {
				return nil, err
			}
			flush()
			tpl.parts = append(tpl.parts, part{tok: tok})
			i = end
		case '}':
			if i+1 < len(rs) && rs[i+1] == '}' {
				lit.WriteRune('}')
				i++
				continue
			}
			return nil, posErr(apperr.TokenUnexpectedClose, i, "} without {")
		default:
			lit.WriteRune(rs[i])
		}
	}
	flush()
	return tpl, nil
}

// Render parses template and renders it at t in lang.
func Render(template string, t time.Time, lang i18n.Lang) (string, error) {
	tpl, err := Parse(template)
	if err != nil {
		return "", err
	}
	return tpl.Render(t, lang), nil
}

// Render fills the tokens with t (in t's location) and lang's names.
func (tpl *Template) Render(t time.Time, lang i18n.Lang) string {
	var b strings.Builder
	for _, p := range tpl.parts {
		if p.tok == nil {
			b.WriteString(p.text)
			continue
		}
		p.tok.render(&b, t, lang)
	}
	return b.String()
}

// HasTokens reports whether the template contains at least one token (a
// template without tokens renders to the same text every time).
func (tpl *Template) HasTokens() bool {
	for _, p := range tpl.parts {
		if p.tok != nil {
			return true
		}
	}
	return false
}

// parseToken parses the content between the braces; open is the index of `{`.
func parseToken(c []rune, open int) (*token, error) {
	if strings.TrimSpace(string(c)) == "" {
		return nil, posErr(apperr.TokenEmpty, open, "empty token")
	}
	base := open + 1 // index in the template of c[0]
	head, format, hasFormat := c, []rune(nil), false
	colon := -1
	for k, r := range c {
		if r == ':' {
			colon = k
			break
		}
	}
	if colon >= 0 {
		head, format, hasFormat = c[:colon], c[colon+1:], true
	}

	nameEnd := 0
	for nameEnd < len(head) && !isModStart(head[nameEnd]) {
		nameEnd++
	}
	name := string(head[:nameEnd])
	if !kinds[name] {
		return nil, posErr(apperr.TokenUnknown, base, "unknown token "+strconv.Quote(name)).With("name", name)
	}

	tok := &token{}
	for k := nameEnd; k < len(head); {
		m, n, err := parseModifier(head[k:])
		if err != nil {
			return nil, posErr(apperr.TokenOffsetInvalid, base+k, err.Error()).With("offset", string(head[k:k+n]))
		}
		tok.mods = append(tok.mods, m)
		k += n
	}

	if !hasFormat {
		f, err := parseFormat([]rune(DefaultFormat))
		if err != nil {
			return nil, fmt.Errorf("default format: %w", err) // a bug: covered by the tests
		}
		tok.format = f
		return tok, nil
	}
	if len(format) == 0 {
		return nil, posErr(apperr.TokenFormatInvalid, base+colon, "empty format").With("format", "")
	}
	f, err := parseFormat(format)
	if err != nil {
		var bad *badLetter
		at := base + colon + 1
		if errors.As(err, &bad) {
			at += bad.at
		}
		return nil, posErr(apperr.TokenFormatInvalid, at, err.Error()).With("format", string(format))
	}
	tok.format = f
	return tok, nil
}

func isModStart(r rune) bool { return r == '+' || r == '-' || r == '@' }

// posErr builds a token error; i is the 0-based rune index, pos is 1-based.
func posErr(code apperr.Code, i int, msg string) *apperr.Error {
	pos := strconv.Itoa(i + 1)
	return apperr.New(code, errors.New(msg+" at "+pos)).With("pos", pos)
}

func (tk *token) render(b *strings.Builder, t time.Time, lang i18n.Lang) {
	y, m, d := t.Date()
	day := civil{y, m, d}
	for _, mod := range tk.mods {
		day = mod.apply(day)
	}
	for _, f := range tk.format {
		f.write(b, day, t, lang)
	}
}
