package xlsxsnapshot

import "strings"

// refSeparators end an unquoted sheet name walking back from its "!".
const refSeparators = " ,;()=+-*/^&<>{}%"

// sheetRefs returns the sheet names a formula (a defined name's RefersTo)
// points at: "Sheet1!A1", "'My sheet'!A1" (a doubled quote is a quote inside
// a quoted name), "Sheet1:Sheet3!A1" (both ends). References to other workbooks
// ("[1]Sheet1!A1") are not returned. Text inside string literals is skipped.
func sheetRefs(formula string) []string {
	rs := []rune(formula)
	var out []string
	inString := false
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '"':
			inString = !inString
		case inString || rs[i] != '!':
		default:
			name, external := sheetBefore(rs, i)
			if name == "" || external {
				continue
			}
			out = append(out, strings.Split(name, ":")...)
		}
	}
	return out
}

// sheetBefore reads the sheet reference that ends at the "!" at i.
func sheetBefore(rs []rune, i int) (string, bool) {
	var name string
	start := i
	if i > 0 && rs[i-1] == '\'' {
		var b []rune
		j := i - 2
		for ; j >= 0; j-- {
			if rs[j] == '\'' {
				if j > 0 && rs[j-1] == '\'' { // an escaped quote
					b = append(b, '\'')
					j--
					continue
				}
				break
			}
			b = append(b, rs[j])
		}
		for l, r := 0, len(b)-1; l < r; l, r = l+1, r-1 {
			b[l], b[r] = b[r], b[l]
		}
		name, start = string(b), j
	} else {
		j := i - 1
		for j >= 0 && !strings.ContainsRune(refSeparators, rs[j]) {
			j--
		}
		name, start = string(rs[j+1:i]), j+1
	}
	// Sheet names can't contain brackets: one means another workbook.
	external := strings.ContainsAny(name, "[]") || (start > 0 && rs[start-1] == ']')
	return name, external
}

// refersToAny reports whether formula points at one of the sheets (names
// lowercased) or holds a broken reference (#REF!).
func refersToAny(formula string, sheets map[string]bool) bool {
	if strings.Contains(strings.ToUpper(formula), "#REF!") {
		return true
	}
	for _, s := range sheetRefs(formula) {
		if sheets[strings.ToLower(s)] {
			return true
		}
	}
	return false
}
