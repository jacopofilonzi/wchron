package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

// Entry is one parsed log record.
type Entry struct {
	Time  time.Time      `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs"`
}

// Filter selects the entries returned by Read.
type Filter struct {
	// Level is the minimum level: "debug", "info", "warn", "error"; "" for all.
	Level string
	// Limit is the maximum number of entries (DefaultLimit when <= 0, at most MaxLimit).
	Limit int
}

// Limits of Read.
const (
	DefaultLimit = 200
	MaxLimit     = 5000
)

// Read returns the most recent entries of the log files in dir, newest
// first, across the current file and the rotated ones. Lines that are not
// JSON records (a line cut by a crash) are skipped. A missing dir or file
// gives no entries; any other failure is logs.read_failed.
func Read(dir string, f Filter) ([]Entry, error) {
	minLevel := slog.LevelDebug
	if f.Level != "" {
		if err := minLevel.UnmarshalText([]byte(f.Level)); err != nil {
			return nil, apperr.New(apperr.Internal, fmt.Errorf("log level filter %q: %w", f.Level, err))
		}
	}
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)

	files, err := logFiles(dir)
	if err != nil {
		return nil, apperr.New(apperr.LogsReadFailed, err)
	}
	out := []Entry{}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue // rotated away meanwhile
		}
		if err != nil {
			return nil, apperr.New(apperr.LogsReadFailed, fmt.Errorf("read %s: %w", name, err))
		}
		lines := bytes.Split(data, []byte("\n"))
		for i := len(lines) - 1; i >= 0; i-- {
			e, ok := parse(lines[i])
			if !ok {
				continue
			}
			var lv slog.Level
			if lv.UnmarshalText([]byte(e.Level)) == nil && lv < minLevel {
				continue
			}
			out = append(out, e)
			if len(out) == limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// logFiles lists the log files in dir, newest first: the current file, then
// the rotated ones (lumberjack names them with a sortable timestamp).
func logFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	ext := filepath.Ext(FileName)
	prefix := strings.TrimSuffix(FileName, ext) + "-"
	var current bool
	var rotated []string
	for _, e := range entries {
		n := e.Name()
		switch {
		case e.IsDir():
		case n == FileName:
			current = true
		case strings.HasPrefix(n, prefix) && strings.HasSuffix(n, ext):
			rotated = append(rotated, n)
		}
	}
	slices.Sort(rotated)
	slices.Reverse(rotated)
	if current {
		rotated = slices.Insert(rotated, 0, FileName)
	}
	return rotated, nil
}

func parse(line []byte) (Entry, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return Entry{}, false
	}
	var m map[string]any
	if json.Unmarshal(line, &m) != nil {
		return Entry{}, false
	}
	e := Entry{Attrs: map[string]any{}}
	ts, _ := m[slog.TimeKey].(string)
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return Entry{}, false
	}
	e.Time = t
	e.Level, _ = m[slog.LevelKey].(string)
	e.Msg, _ = m[slog.MessageKey].(string)
	for k, v := range m {
		switch k {
		case slog.TimeKey, slog.LevelKey, slog.MessageKey:
		default:
			e.Attrs[k] = v
		}
	}
	return e, true
}
