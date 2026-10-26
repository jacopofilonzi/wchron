// Package logging sets up the app logger (log/slog).
//
// Milestone 1: a text handler on stderr. Rotating files under the data dir,
// the in-app log reader and phone number masking come with milestones 2 and 9.
package logging

import (
	"io"
	"log/slog"
)

// New returns a text logger writing to w at the given level.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}
