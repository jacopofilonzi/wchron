package tokens

import (
	"errors"
	"strconv"
	"time"
)

// civil is a calendar date without time of day or zone: offsets and anchors
// work on it so DST can never move the date (see the package doc).
type civil struct {
	y int
	m time.Month
	d int
}

// norm returns the date normalized (e.g. 32 January → 1 February). The
// UTC noon is only a carrier for the arithmetic, never rendered.
func (c civil) norm() civil {
	y, m, d := time.Date(c.y, c.m, c.d, 12, 0, 0, 0, time.UTC).Date()
	return civil{y, m, d}
}

// weekday is 0 for Monday … 6 for Sunday.
func (c civil) weekday() int {
	return (int(time.Date(c.y, c.m, c.d, 12, 0, 0, 0, time.UTC).Weekday()) + 6) % 7
}

func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 12, 0, 0, 0, time.UTC).Day()
}

// addMonths moves by n months, clamping the day to the target month's last day.
func (c civil) addMonths(n int) civil {
	first := civil{c.y, c.m + time.Month(n), 1}.norm()
	return civil{first.y, first.m, min(c.d, daysIn(first.y, first.m))}
}

type modifier interface {
	apply(civil) civil
}

type offset struct {
	n    int
	unit rune
}

func (o offset) apply(c civil) civil {
	switch o.unit {
	case 'd':
		return civil{c.y, c.m, c.d + o.n}.norm()
	case 'w':
		return civil{c.y, c.m, c.d + 7*o.n}.norm()
	case 'm':
		return c.addMonths(o.n)
	default: // 'y'
		return c.addMonths(12 * o.n)
	}
}

type anchor string

// anchors maps every anchor name to its effect on the date.
var anchors = map[anchor]func(civil) civil{
	"month-start": func(c civil) civil { return civil{c.y, c.m, 1} },
	"month-end":   func(c civil) civil { return civil{c.y, c.m, daysIn(c.y, c.m)} },
	"week-start":  func(c civil) civil { return civil{c.y, c.m, c.d - c.weekday()}.norm() },
	"week-end":    func(c civil) civil { return civil{c.y, c.m, c.d + 6 - c.weekday()}.norm() },
	"year-start":  func(c civil) civil { return civil{c.y, time.January, 1} },
	"year-end":    func(c civil) civil { return civil{c.y, time.December, 31} },
}

func (a anchor) apply(c civil) civil { return anchors[a](c) }

// maxOffsetDigits bounds N: 9999 units is far beyond any sensible template
// and keeps every date in a range time.Date handles without overflow.
const maxOffsetDigits = 4

// parseModifier reads one modifier at the start of s (which begins with `+`,
// `-` or `@`). It returns the modifier and the runes it used; on error the
// count is the extent of the bad modifier, for the error's `offset` param.
func parseModifier(s []rune) (modifier, int, error) {
	if s[0] == '@' {
		n := anchorExtent(s)
		a := anchor(string(s[1:n]))
		if _, ok := anchors[a]; !ok {
			return nil, n, errors.New("unknown anchor")
		}
		return a, n, nil
	}
	n := offsetExtent(s)
	digits := 0
	for 1+digits < n && s[1+digits] >= '0' && s[1+digits] <= '9' {
		digits++
	}
	if digits == 0 || digits > maxOffsetDigits {
		return nil, n, errors.New("offset needs 1 to 4 digits")
	}
	if n != 1+digits+1 {
		return nil, n, errors.New("offset needs one unit: d, w, m or y")
	}
	unit := s[1+digits]
	if unit != 'd' && unit != 'w' && unit != 'm' && unit != 'y' {
		return nil, n, errors.New("unknown offset unit")
	}
	v, err := strconv.Atoi(string(s[1 : 1+digits]))
	if err != nil {
		return nil, n, errors.New("offset is not a number")
	}
	if s[0] == '-' {
		v = -v
	}
	return offset{n: v, unit: unit}, n, nil
}

// offsetExtent is the length of an offset modifier: up to the next modifier.
func offsetExtent(s []rune) int {
	n := 1
	for n < len(s) && !isModStart(s[n]) {
		n++
	}
	return n
}

// anchorExtent is the length of an anchor modifier. Anchor names contain
// `-`, so a `-` ends the anchor only when an offset follows (a digit) or
// when nothing that could continue a name does.
func anchorExtent(s []rune) int {
	n := 1
	for n < len(s) {
		r := s[n]
		if r == '+' || r == '@' {
			break
		}
		if r == '-' && (n+1 >= len(s) || !isLetter(s[n+1])) {
			break
		}
		n++
	}
	return n
}

func isLetter(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
