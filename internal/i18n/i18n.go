// Package i18n translates catalog keys for the Go side (tray, notifications).
//
// The catalog is locales/<lang>.json: one flat object of namespaced keys
// (`tray.*`, `notify.*`, `wa.*`, `error.<code>`; `ui.*` keys are for the
// frontend only), shared with the frontend. Placeholders are `{name}` and are
// filled from params; an unknown placeholder is left as is so a missing param
// shows up instead of vanishing. A key missing in a language falls back to
// English, then to the key itself: the consistency test makes both cases bugs.
package i18n

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jacopofilonzi/wchron/locales"
)

// Lang is a supported UI language code.
type Lang string

const (
	IT Lang = "it"
	EN Lang = "en"
	// Default is the language used before the user picks one.
	Default = IT
)

// Langs lists the supported languages.
var Langs = []Lang{IT, EN}

// Valid reports whether l is a supported language.
func Valid(l Lang) bool {
	for _, x := range Langs {
		if x == l {
			return true
		}
	}
	return false
}

var (
	catalog map[Lang]map[string]string
	loadErr error
)

// The catalog is embedded at build time: failing to parse it is a build defect,
// caught by the tests, so the package keeps working with what it could load and
// main logs LoadError at startup.
func init() {
	catalog, loadErr = Load()
}

// LoadError is the error met while loading the embedded catalog, if any.
func LoadError() error { return loadErr }

// Load parses every supported language from the embedded catalog.
func Load() (map[Lang]map[string]string, error) {
	out := make(map[Lang]map[string]string, len(Langs))
	for _, l := range Langs {
		data, err := locales.FS.ReadFile(string(l) + ".json")
		if err != nil {
			return out, fmt.Errorf("read catalog %s: %w", l, err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			return out, fmt.Errorf("parse catalog %s: %w", l, err)
		}
		out[l] = m
	}
	return out, nil
}

// T returns the text for key in lang with the `{name}` placeholders filled.
func T(lang Lang, key string, params map[string]string) string {
	s, ok := catalog[lang][key]
	if !ok {
		s, ok = catalog[EN][key]
	}
	if !ok {
		s = key
	}
	if len(params) == 0 {
		return s
	}
	pairs := make([]string, 0, len(params)*2)
	for k, v := range params {
		pairs = append(pairs, "{"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(s)
}
