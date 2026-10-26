package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/fsutil"
	"github.com/jacopofilonzi/wchron/internal/migrate"
)

// Loaded is the result of Load.
type Loaded struct {
	State State
	// Exists is false when there is no state.json.
	Exists bool
	// From is the schema_version on disk.
	From int
	// Migrated is true when the file is older than CurrentVersion.
	Migrated bool
	// Corrupt is the state.read_failed warning when the file exists but
	// can't be used: State is then Default() and the caller quarantines the file.
	Corrupt error
}

// Load reads path (see the package doc). The file is never written here.
func Load(path string) (Loaded, error) { return load(path, steps) }

func load(path string, steps []migrate.Step) (Loaded, error) {
	cur := migrate.Current(steps)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Loaded{State: Default(), From: cur}, nil
	}
	if err != nil {
		return corrupt(path, cur, err), nil
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
		return corrupt(path, cur, err), nil
	}
	b, err := json.Marshal(res.Doc)
	if err != nil {
		return corrupt(path, cur, fmt.Errorf("re-encode: %w", err)), nil
	}
	st := Default()
	if err := json.Unmarshal(b, &st); err != nil {
		return corrupt(path, cur, fmt.Errorf("decode: %w", err)), nil
	}
	st.SchemaVersion = CurrentVersion
	st.Normalize()
	return Loaded{State: st, Exists: true, From: res.From, Migrated: res.Migrated}, nil
}

func corrupt(path string, cur int, err error) Loaded {
	return Loaded{
		State:   Default(),
		Exists:  true,
		From:    cur,
		Corrupt: apperr.New(apperr.StateReadFailed, fmt.Errorf("read %s: %w", path, err)).With("path", path),
	}
}

// Quarantine moves a corrupt state file aside, to
// `<path>.corrupt-<YYYYMMDD-HHMMSS>`, and returns the new path.
func Quarantine(path string, now time.Time) (string, error) {
	dst := path + ".corrupt-" + now.Format("20060102-150405")
	if err := os.Rename(path, dst); err != nil {
		return "", fmt.Errorf("move %s aside: %w", path, err)
	}
	return dst, nil
}

// Save writes st to path atomically, at CurrentVersion (state.write_failed).
func Save(path string, st State) error {
	st.SchemaVersion = CurrentVersion
	st.Normalize()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return apperr.New(apperr.StateWriteFailed, fmt.Errorf("encode state: %w", err)).With("path", path)
	}
	if err := fsutil.WriteAtomic(path, append(data, '\n')); err != nil {
		return apperr.New(apperr.StateWriteFailed, err).With("path", path)
	}
	return nil
}
