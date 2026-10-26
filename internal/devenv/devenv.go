// Package devenv reads the development-only environment overrides. The
// installed app needs none of them (its settings live in config.json); they
// point a dev run at a separate environment. Every variable is documented in
// .env.example; an invalid value stops the app at startup (devenv.invalid)
// instead of being silently ignored.
//
//   - WCHRON_DATA_DIR: data dir root (default %APPDATA%\wchron); a relative
//     value is resolved against the working directory.
//   - WCHRON_LOG_LEVEL: debug | info | warn | error (default info).
//   - WCHRON_UPDATE_DISABLED: boolean (strconv.ParseBool), skips the update
//     check (default false; used from milestone 10).
package devenv

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

// Variable names.
const (
	DataDir        = "WCHRON_DATA_DIR"
	LogLevel       = "WCHRON_LOG_LEVEL"
	UpdateDisabled = "WCHRON_UPDATE_DISABLED"
)

// Env is the parsed set of overrides.
type Env struct {
	// DataDir is empty when not overridden.
	DataDir        string
	LogLevel       slog.Level
	UpdateDisabled bool
}

// Load reads the overrides through lookup (os.LookupEnv in the app). An
// empty value counts as unset.
func Load(lookup func(string) (string, bool)) (Env, error) {
	get := func(name string) string {
		v, _ := lookup(name)
		return strings.TrimSpace(v)
	}
	env := Env{DataDir: get(DataDir), LogLevel: slog.LevelInfo}
	if v := get(LogLevel); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			env.LogLevel = slog.LevelDebug
		case "info":
			env.LogLevel = slog.LevelInfo
		case "warn":
			env.LogLevel = slog.LevelWarn
		case "error":
			env.LogLevel = slog.LevelError
		default:
			return Env{}, invalid(LogLevel, v, fmt.Errorf("want debug, info, warn or error"))
		}
	}
	if v := get(UpdateDisabled); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Env{}, invalid(UpdateDisabled, v, err)
		}
		env.UpdateDisabled = b
	}
	return env, nil
}

func invalid(name, value string, err error) error {
	return apperr.New(apperr.DevenvInvalid, fmt.Errorf("%s=%q: %w", name, value, err)).
		With("name", name).With("value", value)
}
