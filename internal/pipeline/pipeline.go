// Package pipeline executes a run of a schedule: it is the scheduler's Runner.
//
// One run, in this order:
//
//  1. Every file spec, in order, is resolved at the slot time
//     (internal/resolver) and each match is processed once (its processor
//     from the registry, on a temp copy under %TEMP%\wchron\<run-id>\). A
//     file that fails (file.*, processor.*, temp.unavailable, token.* of its
//     own caption) is left out for everyone; the others go on. Files are not
//     waited for: a file being written at the slot is copied as it is. The
//     name each recipient sees is the spec's send name (resolver.SendName,
//     rendered at the slot; the processed file's name when empty; numbered
//     " (n)" in match order when one spec yields several files); one that
//     renders to an invalid file name is filespec.send_name_invalid and that
//     file is skipped. Processor warnings (xlsx.sheet_missing,
//     xlsx.no_cached_values) go into the report with the original file name;
//     the file is still sent.
//  2. The slot tolerance is checked once, right before the first send:
//     after the Job's Deadline the run is skipped (slot.late); a long run is
//     never cut halfway. A manual run ("send now") has no deadline.
//  3. Each recipient receives all the files, in order, one after the other.
//     Recipients on different channels are served in parallel; on one channel
//     they are served in the schedule's order, and provider.Manager.Send
//     serializes every send of a channel (also across the concurrent runs of
//     different schedules and the error reports) with a random 2–5 s pause
//     between two sends.
//  4. The report goes to package notify (toast, error recipients, with its
//     repeat rules); the result goes back to the scheduler, which stores it
//     in state.json (last_result, last_error = the most severe problem,
//     problems = all of them).
//
// Captions: the schedule caption (a token template rendered at the slot
// time, in the configured language) goes only on the first file sent to
// each recipient among those without a caption of their own; a file spec
// with its own caption (FileSpec.Caption set, even "") uses it on every
// file it yields; the other files have none. The same text on every channel.
// A caption that fails to render (token.*, normally caught by the editor)
// keeps the files that would carry it from being sent, rather than sending
// them without it.
//
// Per recipient, before its sends: a channel that no longer exists is
// recipient.channel_missing; a recipient stored as not verified "no" is
// send.recipient_invalid and never sent to; a channel not connected at that
// moment is send.not_connected. An unverified recipient ("unknown") on a
// connected channel is checked right before the first send and the result
// stored in config.json: rejected → send.recipient_invalid; a temporary
// failure of the check doesn't stop the send (the service still refuses a
// bad target). Then per file, from the channel's capabilities:
// send.files_unsupported, send.too_large, send.caption_too_long skip that file
// for that recipient only. A failed send (send.*, after the provider's one
// short network retry: never another one here) doesn't stop the others.
//
// Result: ok when every planned send went out (warnings may be listed),
// partial when some did, failed when none did, skipped for slot.late (or a
// schedule deleted meanwhile). Every problem carries `schedule` plus `file`,
// `recipient` and `channel` where they apply.
//
// Shutdown: a cancelled context stops the run between two sends (the send
// in progress finishes or fails on its own); the run is logged as
// interrupted and not notified. The temp copies are removed at the end of
// every run, whatever happened (processor cleanups, then the whole run dir);
// CleanTemp removes what a crash left behind, at startup.
//
// Logs: one structured line per problem (code, params with phone numbers
// masked) and one summary per run. Only this package and notify call
// Channel.Send (AGENTS.md invariant).
package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/notify"
	"github.com/jacopofilonzi/wchron/internal/processor"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/resolver"
	"github.com/jacopofilonzi/wchron/internal/scheduler"
	"github.com/jacopofilonzi/wchron/internal/state"
	"github.com/jacopofilonzi/wchron/internal/tokens"
)

// Channels is the delivery side (provider.Manager).
type Channels interface {
	SendInfo(channelID string) (provider.SendInfo, bool)
	Send(ctx context.Context, channelID string, target json.RawMessage, msg provider.Message) error
	VerifyRecipient(ctx context.Context, channelID string, target json.RawMessage) (config.Verified, json.RawMessage, error)
}

// Store is the configuration the run reads (app.Store).
type Store interface {
	Config() config.Config
}

// Reporter gets the report of every run that wasn't interrupted (notify.Notifier).
type Reporter interface {
	Report(ctx context.Context, r notify.Report) []*apperr.Error
}

// Options are the Pipeline's inputs; Store, Channels, Processors, Clock and
// TempDir are required.
type Options struct {
	Store      Store
	Channels   Channels
	Processors *processor.Registry
	Reporter   Reporter
	Clock      clock.Clock
	// Location is where slot times are shown (time.Local).
	Location *time.Location
	// TempDir is %TEMP%\wchron (platform.Paths.TempDir): one folder per run.
	TempDir string
	Log     *slog.Logger
	// NewRunID names the run's temp folder; nil is timestamp + random.
	NewRunID func() string
	// OnStart and OnFinish bracket every run (tray "sending", run events).
	OnStart  func(scheduler.Job)
	OnFinish func(scheduler.Job, scheduler.Result)
}

// Pipeline runs the schedules. Safe for concurrent runs.
type Pipeline struct{ o Options }

var _ scheduler.Runner = (*Pipeline)(nil)

// New returns a Pipeline.
func New(o Options) *Pipeline {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.NewRunID == nil {
		o.NewRunID = func() string {
			b := make([]byte, 3)
			_, _ = rand.Read(b) // never fails (Go 1.24+)
			return o.Clock.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
		}
	}
	return &Pipeline{o: o}
}

// warnings are the codes that don't make a send fail (the file still went
// out, or the problem is only in the report).
var warnings = map[apperr.Code]bool{
	apperr.NotifyFailed:       true,
	apperr.XlsxSheetMissing:   true,
	apperr.XlsxNoCachedValues: true,
}

// IsWarning reports whether a problem code is only a warning.
func IsWarning(c apperr.Code) bool { return warnings[c] }

// CleanTemp removes every leftover run folder (a crash, a kill): call it at
// startup, before the scheduler starts.
func (p *Pipeline) CleanTemp() {
	entries, err := os.ReadDir(p.o.TempDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			p.o.Log.Warn("pipeline: list temp dir", "dir", p.o.TempDir, "err", err)
		}
		return
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(p.o.TempDir, e.Name())); err != nil {
			p.o.Log.Warn("pipeline: remove leftover temp copy", "path", e.Name(), "err", err)
		}
	}
}

// item is one processed file ready to send.
type item struct {
	path, name string
	size       int64
	// caption is the file's own caption (spec override); nil inherits the
	// schedule caption on the first file sent.
	caption *string
}

// run is the state of one run.
type run struct {
	p        *Pipeline
	job      scheduler.Job
	name     string
	mu       sync.Mutex
	problems []*apperr.Error
	planned  int
	sent     int
}

func (r *run) add(e *apperr.Error) {
	e = e.With("schedule", r.name)
	r.mu.Lock()
	r.problems = append(r.problems, e)
	r.mu.Unlock()
	r.p.o.Log.Warn("pipeline: problem", "schedule", r.job.ScheduleID, "code", string(e.Code), "params", maskParams(e.Params), "err", e.Err)
}

// Run executes job (see the package doc).
func (p *Pipeline) Run(ctx context.Context, job scheduler.Job) (res scheduler.Result) {
	if p.o.OnStart != nil {
		p.o.OnStart(job)
	}
	if p.o.OnFinish != nil {
		defer func() { p.o.OnFinish(job, res) }()
	}
	cfg := p.o.Store.Config()
	i := -1
	for k := range cfg.Schedules {
		if cfg.Schedules[k].ID == job.ScheduleID {
			i = k
		}
	}
	if i < 0 {
		e := apperr.New(apperr.ScheduleNotFound, errors.New("deleted before its run")).With("id", job.ScheduleID)
		return scheduler.Result{Status: state.ResultSkipped, Err: e, Problems: []*apperr.Error{e}}
	}
	sc := cfg.Schedules[i]
	r := &run{p: p, job: job, name: sc.Name}
	lang := cfg.Settings.Language
	start := p.o.Clock.Now()

	runDir := filepath.Join(p.o.TempDir, p.o.NewRunID())
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		r.add(apperr.New(apperr.TempUnavailable, fmt.Errorf("create run dir: %w", err)))
		return p.finish(ctx, r, sc)
	}
	defer func() {
		if err := os.RemoveAll(runDir); err != nil {
			p.o.Log.Warn("pipeline: remove temp copies", "dir", runDir, "err", err)
		}
	}()

	items, cleanups := p.prepare(ctx, r, sc, runDir, lang)
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()
	if ctx.Err() != nil {
		return p.finish(ctx, r, sc)
	}

	if !job.Manual && !job.Deadline.IsZero() && p.o.Clock.Now().After(job.Deadline) {
		late := apperr.New(apperr.SlotLate, fmt.Errorf("first send %s after the slot", p.o.Clock.Now().Sub(job.Slot).Round(time.Second))).
			With("schedule", sc.Name).With("slot", scheduler.SlotText(job.Slot.In(p.o.Location)))
		p.o.Log.Info("pipeline: run skipped, too late for the first send", "schedule", sc.ID, "code", string(late.Code))
		return scheduler.Result{Status: state.ResultSkipped, Err: late, Problems: append([]*apperr.Error{late}, r.problems...)}
	}

	var schedCaption *string
	if len(items) > 0 {
		text, err := tokens.Render(sc.Caption, job.Slot, lang)
		if err != nil {
			// A broken caption: the files that would carry it are not sent
			// rather than sent without it.
			r.add(apperr.As(err))
		} else {
			schedCaption = &text
		}
	}
	p.deliver(ctx, r, sc, items, schedCaption)
	res = p.finish(ctx, r, sc)
	p.o.Log.Info("pipeline: run finished", "schedule", sc.ID, "manual", job.Manual, "status", res.Status,
		"sent", r.sent, "planned", r.planned, "problems", len(res.Problems), "took", p.o.Clock.Now().Sub(start).Round(time.Millisecond))
	return res
}

// prepare resolves and processes every file spec once.
func (p *Pipeline) prepare(ctx context.Context, r *run, sc config.Schedule, runDir string, lang i18n.Lang) ([]item, []func()) {
	var items []item
	var cleanups []func()
	for i, spec := range sc.Files {
		if ctx.Err() != nil {
			break
		}
		matches, err := resolver.Resolve(spec, r.job.Slot, lang)
		if err != nil {
			r.add(apperr.As(err))
			continue
		}
		proc, err := p.o.Processors.Get(spec.Processor)
		if err == nil {
			err = proc.Validate(spec.ProcessorOptions)
		}
		if err != nil {
			r.add(apperr.As(err))
			continue
		}
		var own *string
		if spec.Caption != nil {
			text, err := tokens.Render(*spec.Caption, r.job.Slot, lang)
			if err != nil {
				r.add(apperr.As(err))
				continue
			}
			own = &text
		}
		for j, m := range matches {
			dir := filepath.Join(runDir, strconv.Itoa(i)+"-"+strconv.Itoa(j))
			if err := os.MkdirAll(dir, 0o700); err != nil {
				r.add(apperr.New(apperr.TempUnavailable, fmt.Errorf("create file dir: %w", err)))
				continue
			}
			out, cleanup, err := proc.Process(ctx, processor.Source{Path: m.Path, Name: m.Name}, spec.ProcessorOptions,
				processor.Env{TmpDir: dir, At: r.job.Slot, Lang: lang})
			if err != nil {
				if ctx.Err() != nil {
					break
				}
				e := apperr.As(err)
				if e.Params["file"] == "" {
					e = e.With("file", m.Name)
				}
				r.add(e)
				continue
			}
			cleanups = append(cleanups, cleanup)
			name, err := resolver.SendNameNumbered(spec.SendName, out.Name, r.job.Slot, lang, j+1, len(matches))
			if err != nil {
				e := apperr.As(err)
				if e.Params["file"] == "" {
					e = e.With("file", m.Name)
				}
				r.add(e)
				continue
			}
			for _, w := range out.Warnings {
				r.add(w.With("file", m.Name))
			}
			info, err := os.Stat(out.Path)
			if err != nil {
				r.add(apperr.New(apperr.TempUnavailable, fmt.Errorf("stat processed copy: %w", err)).With("file", m.Name))
				continue
			}
			items = append(items, item{path: out.Path, name: name, size: info.Size(), caption: own})
		}
	}
	return items, cleanups
}

// deliver sends the items to every recipient: channels in parallel,
// recipients of one channel in order.
func (p *Pipeline) deliver(ctx context.Context, r *run, sc config.Schedule, items []item, caption *string) {
	if len(items) == 0 {
		return
	}
	var order []string
	groups := map[string][]config.Recipient{}
	for _, rc := range sc.Recipients {
		if _, ok := groups[rc.ChannelID]; !ok {
			order = append(order, rc.ChannelID)
		}
		groups[rc.ChannelID] = append(groups[rc.ChannelID], rc)
	}
	var wg sync.WaitGroup
	for _, chID := range order {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, rc := range groups[chID] {
				if ctx.Err() != nil {
					return
				}
				p.toRecipient(ctx, r, rc, items, caption)
			}
		}()
	}
	wg.Wait()
}

// toRecipient sends every item to one recipient, in order.
func (p *Pipeline) toRecipient(ctx context.Context, r *run, rc config.Recipient, items []item, caption *string) {
	r.mu.Lock()
	r.planned += len(items)
	r.mu.Unlock()
	label := rc.DisplayName
	if label == "" {
		label = rc.ID
	}
	info, ok := p.o.Channels.SendInfo(rc.ChannelID)
	add := func(e *apperr.Error) {
		e = e.With("recipient", label)
		if ok {
			e = e.With("channel", info.Name)
		}
		r.add(e)
	}
	switch {
	case !ok:
		add(apperr.New(apperr.RecipientChannelMissing, fmt.Errorf("channel %q not configured", rc.ChannelID)))
		return
	case rc.Verified == config.VerifiedNo:
		add(apperr.New(apperr.SendRecipientInvalid, errors.New("recipient failed the check (verified: no)")))
		return
	case !info.Connected:
		add(apperr.New(apperr.SendNotConnected, errors.New("channel not connected at send time")))
		return
	case !info.Capabilities.Files:
		add(apperr.New(apperr.SendFilesUnsupported, errors.New("the channel can't send files")))
		return
	}
	target := rc.Target
	if rc.Verified == config.VerifiedUnknown {
		v, norm, err := p.o.Channels.VerifyRecipient(ctx, rc.ChannelID, rc.Target)
		switch v {
		case config.VerifiedNo:
			add(apperr.New(apperr.SendRecipientInvalid, errors.New("rejected by the check before the send")))
			return
		case config.VerifiedYes:
			target = norm
		default:
			p.o.Log.Info("pipeline: recipient check unavailable, sending anyway", "schedule", r.job.ScheduleID,
				"recipient", rc.ID, "err", err)
		}
	}
	caps := info.Capabilities
	first := true
	for _, it := range items {
		if ctx.Err() != nil {
			return
		}
		text := ""
		switch {
		case it.caption != nil:
			text = *it.caption
		case first && caption != nil:
			text = *caption
		case first:
			// The schedule caption is broken: this file would carry it.
			continue
		}
		if caps.MaxFileSize > 0 && it.size > caps.MaxFileSize {
			add(apperr.New(apperr.SendTooLarge, fmt.Errorf("%d bytes over the limit", it.size)).With("file", it.name).
				With("size", formatSize(it.size)).With("max", formatSize(caps.MaxFileSize)))
			continue
		}
		if caps.MaxCaptionLen > 0 && utf8.RuneCountInString(text) > caps.MaxCaptionLen {
			add(apperr.New(apperr.SendCaptionTooLong, fmt.Errorf("%d characters", utf8.RuneCountInString(text))).
				With("file", it.name).With("max", strconv.Itoa(caps.MaxCaptionLen)))
			continue
		}
		err := p.o.Channels.Send(ctx, rc.ChannelID, target, provider.Message{Text: text,
			Attachments: []provider.Attachment{{Path: it.path, Name: it.name}}})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e := apperr.As(err)
			if e.Params["file"] == "" {
				e = e.With("file", it.name)
			}
			add(e)
			continue
		}
		if it.caption == nil {
			first = false
		}
		r.mu.Lock()
		r.sent++
		r.mu.Unlock()
	}
}

// finish computes the result and reports it.
func (p *Pipeline) finish(ctx context.Context, r *run, sc config.Schedule) scheduler.Result {
	r.mu.Lock()
	problems := append([]*apperr.Error(nil), r.problems...)
	sent, planned := r.sent, r.planned
	r.mu.Unlock()
	var worst, firstWarn *apperr.Error
	for _, e := range problems {
		if IsWarning(e.Code) {
			if firstWarn == nil {
				firstWarn = e
			}
		} else if worst == nil {
			worst = e
		}
	}
	res := scheduler.Result{Problems: problems}
	interrupted := ctx.Err() != nil && (sent < planned || planned == 0)
	switch {
	case interrupted && sent > 0:
		res.Status = state.ResultPartial
	case interrupted:
		res.Status = state.ResultFailed
	case worst == nil:
		res.Status = state.ResultOK
	case sent > 0:
		res.Status = state.ResultPartial
	default:
		res.Status = state.ResultFailed
	}
	res.Err = worst
	if res.Err == nil {
		res.Err = firstWarn
	}
	if ctx.Err() != nil {
		p.o.Log.Warn("pipeline: run interrupted by shutdown", "schedule", sc.ID, "sent", sent, "planned", planned)
		return res
	}
	if p.o.Reporter != nil {
		fails := p.o.Reporter.Report(ctx, notify.Report{ScheduleID: sc.ID, ScheduleName: sc.Name, Slot: r.job.Slot,
			Manual: r.job.Manual, Status: res.Status, Problems: problems})
		for _, f := range fails {
			res.Problems = append(res.Problems, f.With("schedule", sc.Name))
		}
		if res.Err == nil && len(fails) > 0 {
			res.Err = fails[0]
		}
	}
	return res
}

// maskParams hides phone numbers in the params for the logs.
func maskParams(ps map[string]string) map[string]string {
	out := make(map[string]string, len(ps))
	for k, v := range ps {
		if k == "recipient" && digits(v) >= 6 {
			v = logging.MaskPhone(v)
		}
		out[k] = v
	}
	return out
}

func digits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// formatSize is a size for the texts: "12.3 MB", "512 KB", "800 B".
func formatSize(n int64) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatInt(n>>10, 10) + " KB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}
