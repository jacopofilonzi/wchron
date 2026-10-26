package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

// AppDirName is the folder of the app under the user config and temp dirs.
const AppDirName = "wchron"

// Paths locates the data dir and its parts. Every path is absolute.
//
// Layout (see STRUCTURE.md "Data and state"): config.json and state.json at
// the root, channels/<id>/, logs/, backups/; temp copies of the processors in
// a separate temp dir (%TEMP%\wchron), never under the data dir.
type Paths interface {
	DataDir() string
	ConfigFile() string
	StateFile() string
	ChannelsDir() string
	LogsDir() string
	BackupsDir() string
	TempDir() string
	// Ensure creates the directories and checks that the data dir is
	// writable (storage.unavailable otherwise).
	Ensure() error
}

// NewPaths returns the paths for this user: the data dir is
// os.UserConfigDir()/wchron (%APPDATA%\wchron on Windows) unless override
// (WCHRON_DATA_DIR) is set; a relative override is resolved against the
// working directory.
func NewPaths(override string) (Paths, error) {
	cwd, err := os.Getwd()
	if err != nil && override != "" && !filepath.IsAbs(override) {
		return nil, apperr.New(apperr.StorageUnavailable, fmt.Errorf("working directory: %w", err)).With("path", override)
	}
	cfg := ""
	if override == "" {
		if cfg, err = os.UserConfigDir(); err != nil {
			return nil, apperr.New(apperr.StorageUnavailable, fmt.Errorf("user config dir: %w", err)).With("path", "%APPDATA%")
		}
	}
	return newPaths(override, cfg, os.TempDir(), cwd), nil
}

func newPaths(override, userConfigDir, tempDir, cwd string) dirPaths {
	data := filepath.Join(userConfigDir, AppDirName)
	if override != "" {
		data = override
		if !filepath.IsAbs(data) {
			data = filepath.Join(cwd, data)
		}
	}
	return dirPaths{data: filepath.Clean(data), temp: filepath.Join(tempDir, AppDirName)}
}

type dirPaths struct{ data, temp string }

func (p dirPaths) DataDir() string     { return p.data }
func (p dirPaths) ConfigFile() string  { return filepath.Join(p.data, "config.json") }
func (p dirPaths) StateFile() string   { return filepath.Join(p.data, "state.json") }
func (p dirPaths) ChannelsDir() string { return filepath.Join(p.data, "channels") }
func (p dirPaths) LogsDir() string     { return filepath.Join(p.data, "logs") }
func (p dirPaths) BackupsDir() string  { return filepath.Join(p.data, "backups") }
func (p dirPaths) TempDir() string     { return p.temp }

func (p dirPaths) Ensure() error {
	for _, d := range []string{p.data, p.ChannelsDir(), p.LogsDir(), p.BackupsDir(), p.temp} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return apperr.New(apperr.StorageUnavailable, fmt.Errorf("create %s: %w", d, err)).With("path", d)
		}
	}
	probe, err := os.CreateTemp(p.data, ".write-check-*")
	if err != nil {
		return apperr.New(apperr.StorageUnavailable, fmt.Errorf("write in %s: %w", p.data, err)).With("path", p.data)
	}
	name := probe.Name()
	if err := errors.Join(probe.Close(), os.Remove(name)); err != nil {
		return apperr.New(apperr.StorageUnavailable, fmt.Errorf("write check in %s: %w", p.data, err)).With("path", p.data)
	}
	return nil
}
