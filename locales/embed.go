// Package locales embeds the shared i18n catalog (it.json, en.json), used by
// internal/i18n in Go and imported directly by the frontend. Keys are flat and
// namespaced (`ui.*`, `tray.*`, `error.<code>`…); en.json is the reference set.
package locales

import "embed"

// FS holds the catalog files, one `<lang>.json` per language.
//
//go:embed *.json
var FS embed.FS
