package devenv

import (
	"log/slog"
	"testing"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestDefaults(t *testing.T) {
	e, err := Load(env(nil))
	if err != nil || e.DataDir != "" || e.LogLevel != slog.LevelInfo || e.UpdateDisabled {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestValues(t *testing.T) {
	e, err := Load(env(map[string]string{DataDir: " ./.devdata ", LogLevel: "DEBUG", UpdateDisabled: "true"}))
	if err != nil || e.DataDir != "./.devdata" || e.LogLevel != slog.LevelDebug || !e.UpdateDisabled {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestInvalid(t *testing.T) {
	for _, m := range []map[string]string{{LogLevel: "verbose"}, {UpdateDisabled: "yes please"}} {
		_, err := Load(env(m))
		if e := apperr.As(err); e.Code != apperr.DevenvInvalid || e.Params["name"] == "" {
			t.Errorf("%v: err %v", m, err)
		}
	}
}
