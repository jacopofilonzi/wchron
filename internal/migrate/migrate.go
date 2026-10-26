// Package migrate is the schema migration framework shared by config.json and
// state.json.
//
// A file format has a version, `schema_version` (an integer >= 1). Version 1
// is the initial format. Each package keeps an ordered, append-only list of
// Steps: steps[i] turns a version i+1 document into a version i+2 one, so the
// current version is len(steps)+1. Steps work on the raw JSON object
// (map[string]any, numbers as json.Number), not on the Go types, because the Go types always describe
// the current version only.
//
// Apply reads the version, refuses a document newer than the app (NewerError:
// the caller must never overwrite it), runs the missing steps in order and
// stamps the new `schema_version` after each one. A missing `schema_version`
// counts as version 1 (a hand-written file).
//
// Rules for the step lists (see AGENTS.md "Upgrading never loses data"):
// never edit or reorder a released step (pushed to main); a new format is a
// new step appended at the end; a step that touches existing data gets a test
// starting from a realistic file at the previous version.
package migrate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// VersionKey is the field holding the format version in every data file.
const VersionKey = "schema_version"

// Step migrates doc in place from version N to N+1.
type Step func(doc map[string]any) error

// Current is the version produced by the given steps.
func Current(steps []Step) int { return len(steps) + 1 }

// Result is a document at the current version.
type Result struct {
	Doc map[string]any
	// From is the version the document had on disk.
	From int
	// Migrated is true when at least one step ran.
	Migrated bool
}

// NewerError: the document was written by a newer app.
type NewerError struct{ Found, Supported int }

func (e *NewerError) Error() string {
	return fmt.Sprintf("schema_version %d is newer than the supported %d", e.Found, e.Supported)
}

// StepError: a migration step failed.
type StepError struct {
	From, To int
	Err      error
}

func (e *StepError) Error() string {
	return fmt.Sprintf("migrate %d to %d: %v", e.From, e.To, e.Err)
}

func (e *StepError) Unwrap() error { return e.Err }

// ErrFormat marks a document that is not a JSON object or has an invalid version.
var ErrFormat = errors.New("invalid data file")

// Apply parses data and brings it to Current(steps).
func Apply(data []byte, steps []Step) (Result, error) {
	// UseNumber keeps numbers exact (a provider id may not fit a float64).
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrFormat, err)
	}
	if dec.More() {
		return Result{}, fmt.Errorf("%w: data after the JSON object", ErrFormat)
	}
	if doc == nil {
		return Result{}, fmt.Errorf("%w: not a JSON object", ErrFormat)
	}
	from, err := Version(doc)
	if err != nil {
		return Result{}, err
	}
	cur := Current(steps)
	if from > cur {
		return Result{}, &NewerError{Found: from, Supported: cur}
	}
	for v := from; v < cur; v++ {
		if err := steps[v-1](doc); err != nil {
			return Result{}, &StepError{From: v, To: v + 1, Err: err}
		}
		doc[VersionKey] = v + 1
	}
	doc[VersionKey] = cur
	return Result{Doc: doc, From: from, Migrated: from < cur}, nil
}

// Version reads `schema_version` from doc (1 when missing).
func Version(doc map[string]any) (int, error) {
	raw, ok := doc[VersionKey]
	if !ok {
		return 1, nil
	}
	n, ok := raw.(json.Number)
	if !ok {
		return 0, fmt.Errorf("%w: %s is %v", ErrFormat, VersionKey, raw)
	}
	v, err := n.Int64()
	if err != nil || v < 1 || v > math.MaxInt32 {
		return 0, fmt.Errorf("%w: %s is %v", ErrFormat, VersionKey, raw)
	}
	return int(v), nil
}
