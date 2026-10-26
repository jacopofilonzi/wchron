package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/fsutil"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/migrate"
)

// Loaded is the result of Load.
type Loaded struct {
	Config Config
	// Exists is false on a fresh install (no config.json): Config is Default().
	Exists bool
	// From is the schema_version on disk (CurrentVersion when !Exists).
	From int
	// Migrated is true when the file is older than CurrentVersion: the
	// caller backs the data up, then saves it.
	Migrated bool
}

// Load reads path (see the package doc for every outcome). The file is never
// written here.
func Load(path string) (Loaded, error) { return load(path, steps) }

func load(path string, steps []migrate.Step) (Loaded, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Loaded{Config: Default(), From: migrate.Current(steps)}, nil
	}
	if err != nil {
		return Loaded{}, readFailed(path, err)
	}
	res, err := migrate.Apply(data, steps)
	var newer *migrate.NewerError
	var step *migrate.StepError
	switch {
	case errors.As(err, &newer):
		return Loaded{}, apperr.New(apperr.ConfigNewerVersion, fmt.Errorf("%s: %w", path, err)).
			With("found", strconv.Itoa(newer.Found)).With("supported", strconv.Itoa(newer.Supported))
	case errors.As(err, &step):
		return Loaded{}, apperr.New(apperr.ConfigMigrationFailed, fmt.Errorf("%s: %w", path, err)).
			With("from", strconv.Itoa(step.From)).With("to", strconv.Itoa(step.To))
	case err != nil:
		return Loaded{}, readFailed(path, err)
	}
	cfg, err := decode(res.Doc)
	if err != nil {
		return Loaded{}, readFailed(path, err)
	}
	if err := cfg.Validate(); err != nil {
		e := apperr.As(err)
		return Loaded{}, e.With("path", path)
	}
	return Loaded{Config: cfg, Exists: true, From: res.From, Migrated: res.Migrated}, nil
}

// decode turns a migrated document into the Go types, on top of the
// defaults so that a field missing in the file keeps its default value.
func decode(doc map[string]any) (Config, error) {
	b, err := json.Marshal(doc)
	if err != nil {
		return Config{}, fmt.Errorf("re-encode: %w", err)
	}
	cfg := Default()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode: %w", err)
	}
	cfg.SchemaVersion = CurrentVersion
	cfg.Normalize()
	return cfg, nil
}

func readFailed(path string, err error) error {
	return apperr.New(apperr.ConfigReadFailed, fmt.Errorf("read %s: %w", path, err)).With("path", path)
}

// Save writes cfg to path atomically, at CurrentVersion.
func Save(path string, cfg Config) error {
	cfg.SchemaVersion = CurrentVersion
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return apperr.As(err).With("path", path)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return apperr.New(apperr.ConfigWriteFailed, fmt.Errorf("encode config: %w", err)).With("path", path)
	}
	if err := fsutil.WriteAtomic(path, append(data, '\n')); err != nil {
		return apperr.New(apperr.ConfigWriteFailed, err).With("path", path)
	}
	return nil
}

// PeekLanguage returns the language stored in path, or the default when the
// file is missing or unreadable. It is meant for the startup error dialog,
// which must speak the user's language even when Load fails.
func PeekLanguage(path string) i18n.Lang {
	data, err := os.ReadFile(path)
	if err != nil {
		return i18n.Default
	}
	var doc struct {
		Settings struct {
			Language i18n.Lang `json:"language"`
		} `json:"settings"`
	}
	if json.Unmarshal(data, &doc) != nil || !i18n.Valid(doc.Settings.Language) {
		return i18n.Default
	}
	return doc.Settings.Language
}
