// Package logging sets up the app logger (log/slog) and reads the log back
// for the in-app viewer.
//
// Format: JSON lines (slog.JSONHandler: `time`, `level`, `msg`, then the
// attributes) in logs/wchron.log under the data dir, so the reader can parse
// them. Rotation (lumberjack): at about MaxSizeMB the file is renamed to
// `wchron-<timestamp>.log` and a new one is started; MaxBackups old files are
// kept (so at most MaxBackups+1 files), never compressed. Dev builds also
// write a text copy to stderr (Options.Stderr).
//
// What never goes into the log raw: phone numbers (MaskPhone keeps the last 3
// digits), session data, tokens, passwords and any other channel secret.
// Error values are logged through their English message; apperr.Error carries
// the code first.
package logging

import (
	"io"
	"log/slog"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Rotation settings.
const (
	FileName   = "wchron.log"
	MaxSizeMB  = 5
	MaxBackups = 4
)

// New returns a text logger writing to w at the given level (used before the
// data dir is known, and in tests).
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// Options configures Open.
type Options struct {
	// Dir is the logs folder (created by platform.Paths.Ensure).
	Dir   string
	Level slog.Level
	// Stderr, when not nil, also gets every record as text (dev builds).
	Stderr io.Writer
}

// Open returns the app logger over the rotating file in opt.Dir, and the
// closer of the file (call it on shutdown).
func Open(opt Options) (*slog.Logger, io.Closer) {
	file := &lumberjack.Logger{
		Filename:   filepath.Join(opt.Dir, FileName),
		MaxSize:    MaxSizeMB,
		MaxBackups: MaxBackups,
		LocalTime:  true,
		Compress:   false,
	}
	ho := &slog.HandlerOptions{Level: opt.Level}
	var h slog.Handler = slog.NewJSONHandler(file, ho)
	if opt.Stderr != nil {
		h = slog.NewMultiHandler(h, slog.NewTextHandler(opt.Stderr, ho))
	}
	return slog.New(h), file
}

// MaskPhone hides a phone number for the logs: only the last 3 digits are
// kept, behind a fixed "***" that doesn't reveal the length, e.g.
// "+39 333 1234567" → "***567". A value with 3 digits or fewer is fully masked.
func MaskPhone(number string) string {
	var digits []rune
	for _, r := range number {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) <= 3 {
		return "***"
	}
	return "***" + string(digits[len(digits)-3:])
}
