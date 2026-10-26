package resolver

// The name a recipient sees (config.FileSpec.SendName).
//
// The template is rendered at the slot time like the path. The result must be
// a valid Windows file name, because recipients save it as one: none of
// < > : " / \ | ? * or control characters, no trailing dot or space, not a
// reserved device name (CON, PRN, AUX, NUL, COM1–9, LPT1–9, also with an
// extension, e.g. "con.txt"), at most 255 characters. Anything else is
// filespec.send_name_invalid (param `value`: the rendered name), at
// validation and at run time alike (there the file is skipped).
//
// The extension of the processed file (".xlsx" for a values-only Excel copy)
// is appended unless the name already has an extension. Dates often carry
// dots ("Report 26.10.2026"), so only a trailing ".abc" made of letters and
// digits with at least one letter counts as an extension: "Report 26.10.2026"
// becomes "Report 26.10.2026.xlsx", "data.csv" stays as it is.

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/tokens"
)

// maxNameLen is the longest file name Windows accepts (NTFS: 255 UTF-16 units;
// counted in characters here, close enough for names typed by hand).
const maxNameLen = 255

// SendName returns the name to send for a processed file named outName: the
// template rendered at t, with outName's extension appended when it has none
// (see above). An empty template keeps outName.
func SendName(template, outName string, t time.Time, lang i18n.Lang) (string, error) {
	if template == "" {
		return outName, nil
	}
	name, err := tokens.Render(template, t, lang)
	if err != nil {
		return "", err
	}
	if ext := filepath.Ext(outName); ext != "" && !hasExtension(name) {
		name += ext
	}
	if err := checkName(name); err != nil {
		return "", err
	}
	return name, nil
}

// ValidateSendName checks a send name template for the editor (token.*,
// filespec.send_name_invalid), rendering it at t. An empty one is valid.
func ValidateSendName(template string, t time.Time, lang i18n.Lang) error {
	if template == "" {
		return nil
	}
	name, err := tokens.Render(template, t, lang)
	if err != nil {
		return err
	}
	return checkName(name)
}

// hasExtension reports whether name ends with ".x" where x is letters and
// digits with at least one letter (so a date's ".2026" doesn't count).
func hasExtension(name string) bool {
	ext := filepath.Ext(name)
	if len(ext) < 2 || len(ext) == len(name) {
		return false
	}
	letter := false
	for _, r := range ext[1:] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letter = true
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return letter
}

var reservedNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

func init() {
	for _, p := range []string{"COM", "LPT"} {
		for i := '1'; i <= '9'; i++ {
			reservedNames[p+string(i)] = true
		}
	}
}

// checkName reports whether name is a valid Windows file name.
func checkName(name string) error {
	bad := func(why string) error {
		return apperr.New(apperr.FilespecSendNameInvalid, fmt.Errorf("send name %q: %s", name, why)).With("value", name)
	}
	if strings.TrimSpace(name) == "" {
		return bad("empty")
	}
	if utf8.RuneCountInString(name) > maxNameLen {
		return bad("too long")
	}
	for _, r := range name {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return bad(fmt.Sprintf("character %q not allowed", r))
		}
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return bad("ends with a dot or a space")
	}
	base, _, _ := strings.Cut(name, ".")
	if reservedNames[strings.ToUpper(strings.TrimRight(base, " "))] {
		return bad("reserved device name")
	}
	if !utf8.ValidString(name) {
		return bad("invalid UTF-8")
	}
	return nil
}
