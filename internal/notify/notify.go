// Package notify fans out the report of a run: a desktop toast and a plain
// text message to each error recipient (settings.error_recipients, on any
// channel; e.g. the user's own WhatsApp number, which lands in the "You"
// chat). A schedule's recipients never get these texts unless the user also
// listed them as error recipients (AGENTS.md invariant).
//
// What is reported: the problems of a run per file and recipient, except
// the skip reasons that are not errors (slot.*: status and log only). A run
// that went fine is silent (the result is in state.json, the Status page and
// the tray).
//
// Repeats: the first failure of a schedule is reported at once. While the
// same problem set persists (same codes for the same files, recipients and
// channels: Signature) it is reported again at most every RepeatEvery (6 h),
// with a line saying it is a repeat; a different problem set is reported at
// once. When the schedule succeeds again after a reported failure, a
// "resolved" notice goes out the same way. The memory is in state.json
// (notified_signature, notified_at per schedule), so it survives a restart.
//
// A manual run ("send now") toasts its problems every time, never writes to
// the error recipients and doesn't touch that memory.
//
// Texts come from the catalog in the configured language (notify.* for the
// frame, error.<code> for each problem line). The toast is shown first; if
// sending to an error recipient fails it is logged, the failure is returned
// as notify.failed (the pipeline lists it with the run's problems) and a
// second toast says so. A channel that is down can't report its own failure:
// the toast covers it. An error recipient stored as not verified "no" is
// skipped.
package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/scheduler"
	"github.com/jacopofilonzi/wchron/internal/state"
)

// RepeatEvery is how often an unchanged problem set is reported again.
const RepeatEvery = 6 * time.Hour

// ToastLines is how many problem lines a toast shows (the rest is counted).
const ToastLines = 4

// Toaster shows a desktop notification (internal/toast in the app, a fake in
// tests). Clicking it opens the window on the Status page.
type Toaster interface {
	Toast(title, body string) error
}

// Sender delivers the messages to the error recipients (provider.Manager).
type Sender interface {
	SendInfo(channelID string) (provider.SendInfo, bool)
	Send(ctx context.Context, channelID string, target json.RawMessage, msg provider.Message) error
}

// Store is the app state notify reads and writes (app.Store).
type Store interface {
	Config() config.Config
	ScheduleState(id string) state.ScheduleState
	UpdateScheduleState(id string, change func(*state.ScheduleState))
}

// Options are the Notifier's inputs; all but Log are required.
type Options struct {
	Toaster Toaster
	Sender  Sender
	Store   Store
	Clock   clock.Clock
	// Location is where slot times are shown (time.Local).
	Location *time.Location
	Log      *slog.Logger
}

// Notifier reports runs. Safe for concurrent use (one report per schedule
// at a time: a schedule never overlaps itself).
type Notifier struct{ o Options }

// New returns a Notifier.
func New(o Options) *Notifier {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.Location == nil {
		o.Location = time.Local
	}
	return &Notifier{o: o}
}

// Report is what a run tells the Notifier.
type Report struct {
	ScheduleID   string
	ScheduleName string
	Slot         time.Time
	Manual       bool
	// Status is a state.Result* value.
	Status   string
	Problems []*apperr.Error
}

// Notifiable returns the problems worth a notification (not the slot.* skip
// reasons, not notify.failed).
func Notifiable(ps []*apperr.Error) []*apperr.Error {
	var out []*apperr.Error
	for _, p := range ps {
		if strings.HasPrefix(string(p.Code), "slot.") || p.Code == apperr.NotifyFailed {
			continue
		}
		out = append(out, p)
	}
	return out
}

// identity are the params that tell one problem from another; the others
// (size, count, pos…) may change between runs without making it new.
var identity = []string{"path", "file", "sheet", "recipient", "channel"}

// Signature identifies a problem set: the same codes for the same files,
// recipients and channels give the same signature, whatever the order.
func Signature(ps []*apperr.Error) string {
	lines := make([]string, 0, len(ps))
	for _, p := range ps {
		var b strings.Builder
		b.WriteString(string(p.Code))
		for _, k := range identity {
			b.WriteString("|" + p.Params[k])
		}
		lines = append(lines, b.String())
	}
	slices.Sort(lines)
	lines = slices.Compact(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:8])
}

// Report sends the notifications of a run (see the package doc) and
// returns the notify.failed problems of the error recipients it couldn't reach.
func (n *Notifier) Report(ctx context.Context, r Report) []*apperr.Error {
	lang := n.o.Store.Config().Settings.Language
	probs := Notifiable(r.Problems)
	slot := scheduler.SlotText(r.Slot.In(n.o.Location))
	params := map[string]string{"schedule": r.ScheduleName, "slot": slot}

	if r.Manual {
		if len(probs) > 0 {
			n.toast(i18n.T(lang, titleKey(r.Status), params), n.body(lang, "notify.header.manual", params, probs, false, ToastLines))
		}
		return nil
	}

	st := n.o.Store.ScheduleState(r.ScheduleID)
	now := n.o.Clock.Now()
	switch {
	case len(probs) > 0:
		sig := Signature(probs)
		repeat := sig == st.NotifiedSignature
		if repeat && now.Sub(st.NotifiedAt) < RepeatEvery {
			n.o.Log.Info("notify: same problems as the last report, not repeated yet", "schedule", r.ScheduleID,
				"since", st.NotifiedAt.Format(time.RFC3339))
			return nil
		}
		n.o.Store.UpdateScheduleState(r.ScheduleID, func(s *state.ScheduleState) {
			s.NotifiedSignature, s.NotifiedAt = sig, now
		})
		title := i18n.T(lang, titleKey(r.Status), params)
		n.toast(title, n.body(lang, "notify.header", params, probs, repeat, ToastLines))
		n.o.Log.Info("notify: problems reported", "schedule", r.ScheduleID, "problems", len(probs), "repeat", repeat)
		return n.toErrorRecipients(ctx, lang, title+"\n"+n.body(lang, "notify.header", params, probs, repeat, 0))
	case r.Status == state.ResultOK && st.NotifiedSignature != "":
		n.o.Store.UpdateScheduleState(r.ScheduleID, func(s *state.ScheduleState) {
			s.NotifiedSignature, s.NotifiedAt = "", time.Time{}
		})
		title := i18n.T(lang, "notify.title.resolved", params)
		body := i18n.T(lang, "notify.resolved", params)
		n.toast(title, body)
		n.o.Log.Info("notify: problems resolved", "schedule", r.ScheduleID)
		return n.toErrorRecipients(ctx, lang, title+"\n"+body)
	}
	return nil
}

func titleKey(status string) string {
	switch status {
	case state.ResultPartial:
		return "notify.title.partial"
	case state.ResultOK:
		return "notify.title.warning"
	default:
		return "notify.title.failed"
	}
}

// body is the header line, one line per problem (at most limit, 0 for all)
// and, for a repeat, the repeat note.
func (n *Notifier) body(lang i18n.Lang, header string, params map[string]string, probs []*apperr.Error, repeat bool, limit int) string {
	lines := []string{i18n.T(lang, header, params)}
	for i, p := range probs {
		if limit > 0 && i == limit && len(probs) > limit+1 {
			lines = append(lines, i18n.T(lang, "notify.more", map[string]string{"count": strconv.Itoa(len(probs) - limit)}))
			break
		}
		lines = append(lines, "• "+i18n.T(lang, "error."+string(p.Code), p.Params))
	}
	if repeat {
		lines = append(lines, i18n.T(lang, "notify.repeat", nil))
	}
	return strings.Join(lines, "\n")
}

func (n *Notifier) toast(title, body string) {
	if err := n.o.Toaster.Toast(title, body); err != nil {
		n.o.Log.Warn("notify: toast failed", "err", err)
	}
}

// toErrorRecipients sends text to every error recipient; the failures come
// back as notify.failed and are shown in a second toast.
func (n *Notifier) toErrorRecipients(ctx context.Context, lang i18n.Lang, text string) []*apperr.Error {
	var failed []*apperr.Error
	for _, r := range n.o.Store.Config().Settings.ErrorRecipients {
		label := r.DisplayName
		if label == "" {
			label = r.ID
		}
		if r.Verified == config.VerifiedNo {
			n.o.Log.Info("notify: error recipient skipped, it failed the check", "recipient", r.ID, "channel", r.ChannelID)
			continue
		}
		var err error
		if _, ok := n.o.Sender.SendInfo(r.ChannelID); !ok {
			err = apperr.New(apperr.RecipientChannelMissing, errors.New("channel missing"))
		} else {
			err = n.o.Sender.Send(ctx, r.ChannelID, r.Target, provider.Message{Text: text})
		}
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return failed
		}
		n.o.Log.Warn("notify: error report not delivered", "code", string(apperr.NotifyFailed), "recipient", r.ID,
			"channel", r.ChannelID, "reason", string(apperr.As(err).Code), "err", err)
		failed = append(failed, apperr.New(apperr.NotifyFailed, fmt.Errorf("send to error recipient %s: %w", r.ID, err)).
			With("recipient", label))
	}
	if len(failed) > 0 {
		lines := make([]string, 0, len(failed))
		for _, f := range failed {
			lines = append(lines, i18n.T(lang, "error."+string(f.Code), f.Params))
		}
		n.toast(i18n.T(lang, "notify.failed.title", nil), strings.Join(lines, "\n"))
	}
	return failed
}
