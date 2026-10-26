package app

// Validation of a schedule for SaveSchedule (and of the error recipients).
//
// Every check of the editor runs here, in Go, so the rules live in one place
// (the GUI only shows the problems): the cadence (scheduler.Validate), the
// recipients (at least one, their channel exists, no duplicates), the caption
// and the per-file captions (tokens), and each file spec (resolver.Validate
// for the path, the processor and its options, the processor's extensions
// against the pattern's, resolver.ValidateSendName). Patterns are rendered
// for now: a file that doesn't exist yet is fine (it may appear by its slot).
//
// Problems are addressed to a field of the editor: "name", "cadence.interval",
// "cadence.anchor", "cadence.window", "cadence.times", "cadence.days",
// "recipients", "recipients.<id>", "caption", "files",
// "files.<id>.path_pattern", "files.<id>.match", "files.<id>.processor",
// "files.<id>.processor_options", "files.<id>.caption",
// "files.<id>.send_name". Items go by id, not index, so a problem stays on
// its row when the user reorders the files.

import (
	"bytes"
	"encoding/json"
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
	"github.com/jacopofilonzi/wchron/internal/tokens"
)

// FieldProblem is a validation problem under one field of the editor.
type FieldProblem struct {
	Field  string            `json:"field"`
	Code   apperr.Code       `json:"code"`
	Params map[string]string `json:"params"`
}

func problem(field string, err error) FieldProblem {
	e := apperr.As(err)
	p := map[string]string{}
	for k, v := range e.Params {
		p[k] = v
	}
	return FieldProblem{Field: field, Code: e.Code, Params: p}
}

// cadenceFields maps the cadence codes to the field they belong under.
var cadenceFields = map[apperr.Code]string{
	apperr.CadenceIntervalInvalid: "cadence.interval",
	apperr.CadenceAnchorInvalid:   "cadence.anchor",
	apperr.CadenceWindowInvalid:   "cadence.window",
	apperr.CadenceWindowEmpty:     "cadence.window",
	apperr.CadenceNoTimes:         "cadence.times",
	apperr.CadenceTimeInvalid:     "cadence.times",
	apperr.CadenceTimeDuplicate:   "cadence.times",
	apperr.ScheduleNoDays:         "cadence.days",
}

// validator holds what the checks need.
type validator struct {
	channels []config.Channel
	procs    *processor.Registry
	now      time.Time
	lang     i18n.Lang
}

// schedule returns every problem of sc (none: it can be saved).
func (v validator) schedule(sc config.Schedule) []FieldProblem {
	var ps []FieldProblem
	add := func(field string, err error) {
		if err != nil {
			ps = append(ps, problem(field, err))
		}
	}
	if strings.TrimSpace(sc.Name) == "" {
		add("name", apperr.New(apperr.ScheduleNameRequired, fmt.Errorf("empty name")))
	}
	if err := v.cadence(sc.Cadence); err != nil {
		e := apperr.As(err)
		field, ok := cadenceFields[e.Code]
		if !ok {
			field = "cadence"
		}
		add(field, e)
	}
	if len(sc.Recipients) == 0 {
		add("recipients", apperr.New(apperr.RecipientRequired, fmt.Errorf("no recipient")))
	}
	ps = append(ps, v.recipients("recipients", sc.Name, sc.Recipients)...)
	_, err := tokens.Render(sc.Caption, v.now, v.lang)
	add("caption", err)
	if len(sc.Files) == 0 {
		add("files", apperr.New(apperr.ScheduleNoFiles, fmt.Errorf("no file")))
	}
	ids := map[string]bool{}
	for _, f := range sc.Files {
		if f.ID == "" || ids[f.ID] {
			add("files", apperr.New(apperr.Internal, fmt.Errorf("empty or duplicate file id %q", f.ID)))
			continue
		}
		ids[f.ID] = true
		ps = append(ps, v.file(f)...)
	}
	return ps
}

// cadence checks the structural enums first (an unknown mode or day can only
// come from a bug in the GUI), then the values.
func (v validator) cadence(c config.Cadence) error {
	if c.Mode != config.CadenceInterval && c.Mode != config.CadenceFixed {
		return apperr.New(apperr.Internal, fmt.Errorf("unknown cadence mode %q", c.Mode))
	}
	for _, d := range c.Days {
		if !slices.Contains(config.Weekdays, d) {
			return apperr.New(apperr.Internal, fmt.Errorf("unknown day %q", d))
		}
	}
	return scheduler.Validate(c)
}

// recipients checks a recipient list (a schedule's, or the error
// recipients with field "error_recipients"): ids, channels, duplicates.
func (v validator) recipients(field, schedule string, rs []config.Recipient) []FieldProblem {
	var ps []FieldProblem
	ids := map[string]bool{}
	seen := map[string]bool{}
	for _, r := range rs {
		f := field + "." + r.ID
		if r.ID == "" || ids[r.ID] {
			ps = append(ps, problem(field, apperr.New(apperr.Internal, fmt.Errorf("empty or duplicate recipient id %q", r.ID))))
			continue
		}
		ids[r.ID] = true
		label := r.DisplayName
		if label == "" {
			label = string(r.Target)
		}
		if !slices.ContainsFunc(v.channels, func(c config.Channel) bool { return c.ID == r.ChannelID }) {
			ps = append(ps, problem(f, apperr.New(apperr.RecipientChannelMissing, fmt.Errorf("channel %q missing", r.ChannelID)).
				With("schedule", schedule).With("recipient", label)))
			continue
		}
		if !slices.Contains(config.VerifiedStates, r.Verified) {
			ps = append(ps, problem(f, apperr.New(apperr.Internal, fmt.Errorf("unknown verified state %q", r.Verified))))
			continue
		}
		key := r.ChannelID + "\x00" + string(compactJSON(r.Target))
		if seen[key] {
			ps = append(ps, problem(f, apperr.New(apperr.RecipientDuplicate, fmt.Errorf("duplicate target")).With("recipient", label)))
			continue
		}
		seen[key] = true
	}
	return ps
}

// file checks one file spec.
func (v validator) file(f config.FileSpec) []FieldProblem {
	var ps []FieldProblem
	pre := "files." + f.ID + "."
	add := func(field string, err error) {
		if err != nil {
			ps = append(ps, problem(pre+field, err))
		}
	}
	rendered, err := resolver.Render(f.PathPattern, v.now, v.lang)
	add("path_pattern", err)
	switch f.Match {
	case config.MatchError, config.MatchNewest, config.MatchAll:
	default:
		add("match", apperr.New(apperr.Internal, fmt.Errorf("unknown match mode %q", f.Match)))
	}
	proc, err := v.procs.Get(f.Processor)
	if err != nil {
		add("processor", err)
	} else {
		add("processor_options", proc.Validate(f.ProcessorOptions))
		if rendered != "" {
			add("processor", extensionCheck(proc, f.PathPattern, rendered))
		}
	}
	if f.Caption != nil {
		_, err := tokens.Render(*f.Caption, v.now, v.lang)
		add("caption", err)
	}
	add("send_name", resolver.ValidateSendName(f.SendName, v.now, v.lang))
	return ps
}

// extensionCheck refuses a processor for a file type it doesn't handle, when
// the pattern fixes the extension (no wildcard and no token in it: those are
// known only at the slot, where the processor itself refuses the file).
func extensionCheck(proc processor.Processor, pattern, rendered string) error {
	exts := proc.Extensions()
	if len(exts) == 0 {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(rendered))
	if strings.ContainsAny(ext, "*?") || strings.ContainsAny(filepath.Ext(pattern), "{}") {
		return nil
	}
	if ext == "" || !slices.Contains(exts, ext) {
		shown := ext
		if shown == "" {
			shown = filepath.Base(rendered)
		}
		return apperr.New(apperr.ProcessorExtension, fmt.Errorf("%s does not handle %q", proc.Type(), ext)).With("ext", shown)
	}
	return nil
}

// compactJSON makes two encodings of the same target compare equal.
func compactJSON(raw json.RawMessage) []byte {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return raw
	}
	return b.Bytes()
}
