package app

// Schedule bindings: the list, the editor (get, save with validation,
// duplicate, delete, enable) and its previews (processors, files, send
// names). Every change is saved atomically through the Store
// (config.write_failed on failure, nothing changes) and then reported
// through the changed hook (main.go: scheduler Reload + EventSchedules).

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/processor"
	"github.com/jacopofilonzi/wchron/internal/resolver"
	"github.com/jacopofilonzi/wchron/internal/scheduler"
)

// EventSchedules tells the frontend that the schedules changed (saved,
// deleted, enabled, a channel removed some recipients): it reloads the list.
const EventSchedules = "schedules:changed"

// SchedulesChanged is the payload of EventSchedules: the schedule that
// changed, "" when several may have.
type SchedulesChanged struct {
	ID string `json:"id"`
}

// Extras are the Service's inputs beyond NewService's, set from main.go with
// Configure (a plain function: exported methods become bindings).
type Extras struct {
	Processors *processor.Registry
	// DataDir and BackupsDir are shown in Settings; LogsDir is NewService's.
	DataDir    string
	BackupsDir string
	// Changed is called after every schedule change (scheduler Reload and
	// EventSchedules in main.go); id is "" when several may have changed.
	Changed func(id string)
	// OpenFolder shows a folder in the file manager (platform.OpenFolder).
	OpenFolder func(path string) error
}

// Configure sets the Service's extras.
func Configure(s *Service, x Extras) {
	if x.Processors != nil {
		s.procs = x.Processors
	}
	s.dataDir, s.backupsDir = x.DataDir, x.BackupsDir
	if x.Changed != nil {
		s.changed = x.Changed
	}
	if x.OpenFolder != nil {
		s.openFolder = x.OpenFolder
	}
}

// ScheduleSummary is a schedule as the list shows it.
type ScheduleSummary struct {
	Schedule config.Schedule `json:"schedule"`
	// LastResult is a state.Result* value, "" if it never ran; LastError the
	// most severe problem or the skip reason.
	LastResult string      `json:"last_result"`
	LastError  *apperr.Ref `json:"last_error"`
	LastRunAt  *time.Time  `json:"last_run_at"`
	// NextRun is the next slot from now (no jitter), null when disabled or
	// the cadence is invalid (Invalid: its code).
	NextRun *time.Time  `json:"next_run"`
	Invalid *apperr.Ref `json:"invalid"`
}

// SaveResult is SaveSchedule's outcome: the saved schedule (with the ids it
// was given), or the problems that kept it from being saved.
type SaveResult struct {
	Schedule *config.Schedule `json:"schedule"`
	Problems []FieldProblem   `json:"problems"`
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	return hex.EncodeToString(b)
}

func (s *Service) lang() i18n.Lang { return s.store.Snapshot().Language }

func (s *Service) validator() validator {
	return validator{channels: s.store.Config().Channels, procs: s.procs, now: s.clock.Now(), lang: s.lang()}
}

func notFound(id string) error {
	return apperr.New(apperr.ScheduleNotFound, fmt.Errorf("schedule %q not found", id)).With("id", id)
}

// ListSchedules returns every schedule in config order, with its last result
// and next run.
func (s *Service) ListSchedules() []ScheduleSummary {
	now := s.clock.Now()
	out := []ScheduleSummary{}
	for _, sc := range s.store.Config().Schedules {
		sum := ScheduleSummary{Schedule: sc}
		st := s.store.ScheduleState(sc.ID)
		sum.LastResult = st.LastResult
		if st.LastError != nil {
			sum.LastError = &apperr.Ref{Code: st.LastError.Code, Params: st.LastError.Params}
		}
		if !st.LastRunAt.IsZero() {
			t := st.LastRunAt
			sum.LastRunAt = &t
		}
		if err := scheduler.Validate(sc.Cadence); err != nil {
			sum.Invalid = apperr.RefOf(err)
		} else if sc.Enabled {
			if next, err := scheduler.Next(sc.Cadence, now, 1, s.loc); err == nil && len(next) == 1 {
				sum.NextRun = &next[0]
			}
		}
		out = append(out, sum)
	}
	return out
}

// GetSchedule returns one schedule (schedule.not_found).
func (s *Service) GetSchedule(id string) (config.Schedule, error) {
	for _, sc := range s.store.Config().Schedules {
		if sc.ID == id {
			return sc, nil
		}
	}
	return config.Schedule{}, notFound(id)
}

// NewSchedule returns the defaults of a new schedule for the editor (not
// saved): enabled, every hour on weekdays, no recipient, no file. Its id is
// empty until SaveSchedule gives it one.
func (s *Service) NewSchedule() config.Schedule {
	c := config.Config{Schedules: []config.Schedule{{
		Enabled: true,
		Cadence: config.Cadence{Mode: config.CadenceInterval, Interval: config.Interval{Hours: 1}, Anchor: "00:00",
			Days: []config.Weekday{"mon", "tue", "wed", "thu", "fri"}},
	}}}
	c.Normalize()
	return c.Schedules[0]
}

// NewFileSpec returns a new file spec for the editor: an id, match "error",
// processor "passthrough".
func (s *Service) NewFileSpec() config.FileSpec {
	c := config.Config{Schedules: []config.Schedule{{Files: []config.FileSpec{{ID: newID()}}}}}
	c.Normalize()
	return c.Schedules[0].Files[0]
}

// NewRecipientID returns a fresh id for a recipient added in the editor.
func (s *Service) NewRecipientID() string { return newID() }

// SaveSchedule validates a schedule and, when it has no problems, saves it:
// an empty id adds it at the end of the list (with a new id), an existing id
// replaces that schedule in place. Problems come back in the result, not as
// an error; errors are config.write_failed and schedule.not_found (an id that
// no longer exists).
func (s *Service) SaveSchedule(sc config.Schedule) (SaveResult, error) {
	wrap := config.Config{Schedules: []config.Schedule{sc}}
	wrap.Normalize()
	sc = wrap.Schedules[0]
	sc.Name = strings.TrimSpace(sc.Name)
	for i := range sc.Files {
		sc.Files[i].PathPattern = strings.TrimSpace(sc.Files[i].PathPattern)
		sc.Files[i].SendName = strings.TrimSpace(sc.Files[i].SendName)
	}
	if ps := s.validator().schedule(sc); len(ps) > 0 {
		return SaveResult{Problems: ps}, nil
	}
	isNew := sc.ID == ""
	if isNew {
		sc.ID = newID()
	}
	err := s.store.UpdateConfig(func(c *config.Config) error {
		i := slices.IndexFunc(c.Schedules, func(x config.Schedule) bool { return x.ID == sc.ID })
		switch {
		case isNew:
			c.Schedules = append(c.Schedules, sc)
		case i < 0:
			return notFound(sc.ID)
		default:
			c.Schedules[i] = sc
		}
		return nil
	})
	if err != nil {
		return SaveResult{}, err
	}
	s.log.Info("schedule saved", "schedule", sc.ID, "new", isNew)
	s.changed(sc.ID)
	return SaveResult{Schedule: &sc, Problems: []FieldProblem{}}, nil
}

// DuplicateSchedule copies a schedule right after the original, disabled,
// with the name "<name> (copy)" in the current language, and returns the copy.
func (s *Service) DuplicateSchedule(id string) (config.Schedule, error) {
	var cp config.Schedule
	lang := s.lang() // before UpdateConfig: it holds the store's lock
	err := s.store.UpdateConfig(func(c *config.Config) error {
		i := slices.IndexFunc(c.Schedules, func(x config.Schedule) bool { return x.ID == id })
		if i < 0 {
			return notFound(id)
		}
		// c is already a deep copy of the configuration, so the copy shares
		// nothing with the original once it is cloned once more below.
		clone, err := cloneConfig(config.Config{Schedules: []config.Schedule{c.Schedules[i]}})
		if err != nil {
			return apperr.New(apperr.Internal, err)
		}
		cp = clone.Schedules[0]
		cp.ID = newID()
		cp.Name = i18n.T(lang, "schedule.copy_name", map[string]string{"name": cp.Name})
		cp.Enabled = false
		c.Schedules = slices.Insert(c.Schedules, i+1, cp)
		return nil
	})
	if err != nil {
		return config.Schedule{}, err
	}
	s.log.Info("schedule duplicated", "schedule", id, "copy", cp.ID)
	s.changed(cp.ID)
	return cp, nil
}

// DeleteSchedule deletes a schedule for good (after the GUI's confirmation)
// and forgets its last-run state.
func (s *Service) DeleteSchedule(id string) error {
	err := s.store.UpdateConfig(func(c *config.Config) error {
		i := slices.IndexFunc(c.Schedules, func(x config.Schedule) bool { return x.ID == id })
		if i < 0 {
			return notFound(id)
		}
		c.Schedules = slices.Delete(c.Schedules, i, i+1)
		return nil
	})
	if err != nil {
		return err
	}
	s.store.DeleteScheduleState(id)
	s.log.Info("schedule deleted", "schedule", id)
	s.changed(id)
	return nil
}

// SetScheduleEnabled turns a schedule on or off. Turning on a schedule that
// can't run (e.g. left without recipients when a channel was deleted)
// returns its first validation problem as the error.
func (s *Service) SetScheduleEnabled(id string, on bool) (config.Schedule, error) {
	sc, err := s.GetSchedule(id)
	if err != nil {
		return config.Schedule{}, err
	}
	if on {
		if ps := s.validator().schedule(sc); len(ps) > 0 {
			e := apperr.New(ps[0].Code, errors.New("schedule can't be enabled: "+ps[0].Field))
			for k, v := range ps[0].Params {
				e = e.With(k, v)
			}
			return config.Schedule{}, e
		}
	}
	err = s.store.UpdateConfig(func(c *config.Config) error {
		i := slices.IndexFunc(c.Schedules, func(x config.Schedule) bool { return x.ID == id })
		if i < 0 {
			return notFound(id)
		}
		c.Schedules[i].Enabled = on
		sc = c.Schedules[i]
		return nil
	})
	if err != nil {
		return config.Schedule{}, err
	}
	s.changed(id)
	return sc, nil
}

// ProcessorSchemas returns every processor's options schema and extensions,
// in registration order.
func (s *Service) ProcessorSchemas() []processor.Schema { return s.procs.Schemas() }

// PreviewFile is one file a pattern resolves to now.
type PreviewFile struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// FilePreview is what a pattern resolves to now: the rendered pattern and the
// matching files, or the problem (filespec.*, token.*, file.not_found,
// file.multiple_matches, file.unreadable) as data, never as an error.
type FilePreview struct {
	Path  string        `json:"path"`
	Files []PreviewFile `json:"files"`
	Error *apperr.Ref   `json:"error"`
}

// PreviewFiles resolves a pattern now (tokens for now, current language)
// with a match mode, for the editor's live preview.
func (s *Service) PreviewFiles(pattern string, match config.MatchMode) FilePreview {
	now, lang := s.clock.Now(), s.lang()
	out := FilePreview{Files: []PreviewFile{}}
	p, err := resolver.Render(pattern, now, lang)
	if err != nil {
		out.Error = apperr.RefOf(err)
		return out
	}
	out.Path = p
	ms, err := resolver.Resolve(config.FileSpec{PathPattern: pattern, Match: match}, now, lang)
	if err != nil {
		out.Error = apperr.RefOf(err)
		return out
	}
	for _, m := range ms {
		out.Files = append(out.Files, PreviewFile{Name: m.Name, Size: m.Size, ModTime: m.ModTime})
	}
	return out
}

// PreviewSendName returns the names the recipients would see now for a file
// spec (send name template, pattern, match mode, processor), with the same
// rules as a run (resolver.SendNameNumbered: extension, numbering of several
// matches). When the pattern matches nothing now, the name is computed for
// the pattern's own file name, so the extension still shows. Errors: token.*,
// filespec.send_name_invalid, processor.unknown.
func (s *Service) PreviewSendName(template, pattern string, match config.MatchMode, processorType string) ([]string, error) {
	proc, err := s.procs.Get(processorType)
	if err != nil {
		return nil, err
	}
	now, lang := s.clock.Now(), s.lang()
	var names []string
	if ms, err := resolver.Resolve(config.FileSpec{PathPattern: pattern, Match: match}, now, lang); err == nil {
		for _, m := range ms {
			names = append(names, m.Name)
		}
	} else if p, rerr := resolver.Render(pattern, now, lang); rerr == nil {
		names = []string{filepath.Base(p)}
	} else {
		names = []string{""}
	}
	out := make([]string, 0, len(names))
	for i, n := range names {
		name, err := resolver.SendNameNumbered(template, proc.OutputName(n), now, lang, i+1, len(names))
		if err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, nil
}
