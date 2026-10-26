// Package resolver turns a file spec's path pattern into the concrete
// file(s) to send at a slot.
//
// Steps: the tokens of the pattern are rendered at the slot time (not at the
// time the run happens to start, so a run delayed by the jitter still sends
// the slot's files), in the configured language; the result must be an
// absolute path (C:\… or \\server\share\…); the wildcards `*` (any run of
// characters) and `?` (one character) are allowed only in the file name,
// never in the folders (filespec.glob_in_dir). No other glob syntax: `[` and
// `]` are plain characters, since they are common in real file names.
// Matching is case-insensitive (Windows file names are) and only regular
// files match, never folders.
//
// Several matches follow the spec's match mode: `error` (the default:
// file.multiple_matches with the count), `newest` (latest modification time;
// ties go to the first name), `all` (every match, sorted by name). No match:
// file.not_found with the rendered pattern, and that file is skipped. A
// folder that can't be listed is file.unreadable.
//
// Files are not waited for: a file being written at the slot is taken as it
// is. Open is the only way the run reads an original: a shared read that
// never disturbs the program that has it open (Excel), mapping a sharing or
// lock violation to file.locked and any other failure to file.unreadable.
// Originals are never modified (processors work on a temp copy).
//
// Errors carry `path` (rendered pattern) or `file` (file name) and `count`;
// the pipeline adds `schedule`.
package resolver

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/platform"
	"github.com/jacopofilonzi/wchron/internal/tokens"
)

// Match is one resolved file.
type Match struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
}

// Render renders the pattern's tokens at t and checks the result: absolute,
// wildcards only in the file name. It returns the concrete pattern.
func Render(pattern string, t time.Time, lang i18n.Lang) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		return "", apperr.New(apperr.FilespecPathRequired, errors.New("empty path"))
	}
	p, err := tokens.Render(pattern, t, lang)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(p) {
		return "", apperr.New(apperr.FilespecPathNotAbsolute, fmt.Errorf("%q is not absolute", p)).With("value", p)
	}
	dir, name := filepath.Split(p)
	if strings.ContainsAny(dir, "*?") {
		return "", apperr.New(apperr.FilespecGlobInDir, fmt.Errorf("wildcard in folder of %q", p)).With("value", p)
	}
	if name == "" {
		return "", apperr.New(apperr.FilespecPathRequired, fmt.Errorf("%q has no file name", p))
	}
	return p, nil
}

// Validate checks a pattern for the editor (filespec.*, token.*), rendering
// it at t.
func Validate(pattern string, t time.Time, lang i18n.Lang) error {
	_, err := Render(pattern, t, lang)
	return err
}

// Resolve returns the files of spec at slot t (see the package doc).
func Resolve(spec config.FileSpec, t time.Time, lang i18n.Lang) ([]Match, error) {
	p, err := Render(spec.PathPattern, t, lang)
	if err != nil {
		return nil, err
	}
	dir, name := filepath.Split(p)
	if !strings.ContainsAny(name, "*?") {
		info, err := os.Stat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()):
			return nil, notFound(p, err)
		case err != nil:
			return nil, apperr.New(apperr.FileUnreadable, fmt.Errorf("stat %s: %w", p, err)).With("file", name)
		}
		return []Match{{Path: p, Name: info.Name(), Size: info.Size(), ModTime: info.ModTime()}}, nil
	}

	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, notFound(p, err)
	case err != nil:
		return nil, apperr.New(apperr.FileUnreadable, fmt.Errorf("list %s: %w", dir, err)).With("file", dir)
	}
	var ms []Match
	for _, e := range entries {
		if !e.Type().IsRegular() || !globMatch(name, e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil { // removed meanwhile
			continue
		}
		ms = append(ms, Match{Path: filepath.Join(dir, e.Name()), Name: e.Name(), Size: info.Size(), ModTime: info.ModTime()})
	}
	slices.SortFunc(ms, func(a, b Match) int { return strings.Compare(a.Name, b.Name) })
	switch {
	case len(ms) == 0:
		return nil, notFound(p, nil)
	case len(ms) == 1 || spec.Match == config.MatchAll:
		return ms, nil
	case spec.Match == config.MatchNewest:
		best := ms[0]
		for _, m := range ms[1:] {
			if m.ModTime.After(best.ModTime) {
				best = m
			}
		}
		return []Match{best}, nil
	default:
		return nil, apperr.New(apperr.FileMultipleMatches, fmt.Errorf("%d files match %s", len(ms), p)).
			With("path", p).With("count", strconv.Itoa(len(ms)))
	}
}

func notFound(p string, err error) error {
	if err == nil {
		err = errors.New("no match")
	}
	return apperr.New(apperr.FileNotFound, fmt.Errorf("%s: %w", p, err)).With("path", p)
}

// Open opens an original for reading without disturbing the program that
// may have it open (file.locked, file.unreadable with the file name).
func Open(path string) (*os.File, error) {
	f, err := platform.OpenShared(path)
	if err != nil {
		return nil, ReadError(path, err)
	}
	return f, nil
}

// ReadError maps a failure to open or read an original to file.locked or
// file.unreadable (param `file`: its name).
func ReadError(path string, err error) error {
	code := apperr.FileUnreadable
	if platform.IsLocked(err) {
		code = apperr.FileLocked
	}
	return apperr.New(code, fmt.Errorf("read %s: %w", path, err)).With("file", filepath.Base(path))
}

// globMatch matches name against pattern with `*` and `?` only,
// case-insensitively.
func globMatch(pattern, name string) bool {
	p := []rune(strings.ToLower(pattern))
	n := []rune(strings.ToLower(name))
	// Iterative wildcard match with backtracking to the last `*`.
	pi, ni, star, mark := 0, 0, -1, 0
	for ni < len(n) {
		switch {
		case pi < len(p) && (p[pi] == '?' || p[pi] == n[ni]):
			pi++
			ni++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, ni
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			ni = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}
