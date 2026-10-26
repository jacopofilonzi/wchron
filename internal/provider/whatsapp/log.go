package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// waLogger adapts whatsmeow's logger to slog (never stdout). whatsmeow's
// info lines are routine (connects, syncs) and go to debug; the "Recv" and
// "Send" modules dump raw protocol nodes, which may hold keys and message
// data, and are dropped. Every line has its phone numbers masked.
type waLogger struct {
	l      *slog.Logger
	module string
}

func newLogger(l *slog.Logger) waLog.Logger {
	return &waLogger{l: l, module: "whatsmeow"}
}

// digitRun matches phone numbers and the user part of JIDs.
var digitRun = regexp.MustCompile(`\+?\d{6,}`)

// maskNumbers keeps only the last 3 digits of every long digit run, like
// logging.MaskPhone.
func maskNumbers(s string) string {
	return digitRun.ReplaceAllStringFunc(s, func(m string) string { return "***" + m[len(m)-3:] })
}

func (w *waLogger) log(level slog.Level, msg string, args []any) {
	if !w.l.Enabled(context.Background(), level) {
		return
	}
	w.l.Log(context.Background(), level, maskNumbers(fmt.Sprintf(msg, args...)), "module", w.module)
}

func (w *waLogger) Errorf(msg string, args ...any) { w.log(slog.LevelError, msg, args) }
func (w *waLogger) Warnf(msg string, args ...any)  { w.log(slog.LevelWarn, msg, args) }
func (w *waLogger) Infof(msg string, args ...any)  { w.log(slog.LevelDebug, msg, args) }
func (w *waLogger) Debugf(msg string, args ...any) { w.log(slog.LevelDebug, msg, args) }

func (w *waLogger) Sub(module string) waLog.Logger {
	if module == "Recv" || module == "Send" {
		return waLog.Noop
	}
	return &waLogger{l: w.l, module: w.module + "/" + module}
}
