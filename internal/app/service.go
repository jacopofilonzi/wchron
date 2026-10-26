package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/scheduler"
	"github.com/jacopofilonzi/wchron/internal/tokens"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventState carries the full Snapshot after every change (backend → frontend),
// including the next scheduled run (Snapshot.NextRun): there is no separate
// schedule event.
const EventState = "app:state"

// EventChannelStatus carries a channel whose status changed (ChannelInfo,
// with its id); EventChannelQR every pairing step (PairingEvent: QR code,
// success, timeout, failure, cancel).
const (
	EventChannelStatus = "channel:status"
	EventChannelQR     = "channel:qr"
)

// EventRunStarted and EventRunFinished bracket every run (RunEvent); the
// finished one carries the status and the problems.
const (
	EventRunStarted  = "run:started"
	EventRunFinished = "run:finished"
)

// RunEvent is a run as the GUI sees it.
type RunEvent struct {
	ScheduleID   string `json:"schedule_id"`
	ScheduleName string `json:"schedule_name"`
	Manual       bool   `json:"manual"`
	// Status is "" for run:started, a state.Result* value for run:finished.
	Status   string       `json:"status"`
	Problems []apperr.Ref `json:"problems"`
}

// Runs starts manual runs (scheduler.Scheduler).
type Runs interface {
	RunNow(id, name string) error
}

// channelCallTimeout bounds the calls that ask the service (target check,
// group list).
const channelCallTimeout = 30 * time.Second

// The Store is the scheduler's view of the app state.
var _ scheduler.Store = (*Store)(nil)

func init() {
	application.RegisterEvent[Snapshot](EventState)
	application.RegisterEvent[provider.ChannelInfo](EventChannelStatus)
	application.RegisterEvent[provider.PairingEvent](EventChannelQR)
	application.RegisterEvent[RunEvent](EventRunStarted)
	application.RegisterEvent[RunEvent](EventRunFinished)
}

// Service is the Wails service bound to the frontend. Each method returns the
// new state, so the caller doesn't have to wait for EventState.
type Service struct {
	store    *Store
	channels *provider.Manager
	registry *provider.Registry
	runs     Runs
	logsDir  string
	clock    clock.Clock
	loc      *time.Location // time.Local; tests set Europe/Rome
}

// NewService wraps the store, the channel manager and the scheduler (runs)
// for the bindings; logsDir is where Logs reads, clk the source of "now" for
// the previews (clock.Real in the app).
func NewService(st *Store, reg *provider.Registry, channels *provider.Manager, runs Runs, logsDir string, clk clock.Clock) *Service {
	return &Service{store: st, registry: reg, channels: channels, runs: runs, logsDir: logsDir, clock: clk, loc: time.Local}
}

// SendNow runs a schedule at once ("send now"), enabled or not, ignoring its
// days, window and tolerance. It returns when the run has started; the
// outcome arrives as EventRunFinished, a toast if there are problems (never a
// message to the error recipients) and the schedule's state. A run of the
// same schedule in progress refuses it (schedule.busy); schedule.not_found
// if it was deleted. Acts for real: it sends to the schedule's recipients.
func (s *Service) SendNow(id string) error {
	for _, sc := range s.store.Schedules() {
		if sc.ID == id {
			return s.runs.RunNow(sc.ID, sc.Name)
		}
	}
	return apperr.New(apperr.ScheduleNotFound, fmt.Errorf("schedule %q not found", id)).With("id", id)
}

// AckErrors clears the tray's error state; the Status page calls it when it opens.
func (s *Service) AckErrors() Snapshot { return s.store.AckErrors() }

// State returns the current state.
func (s *Service) State() Snapshot { return s.store.Snapshot() }

// SetRoutineActive pauses or resumes the routine.
func (s *Service) SetRoutineActive(on bool) (Snapshot, error) { return s.store.SetRoutineActive(on) }

// SetKeepAwake turns keep-awake on or off (keepawake.failed on error).
func (s *Service) SetKeepAwake(on bool) (Snapshot, error) { return s.store.SetKeepAwake(on) }

// SetKeepAwakeDefault sets whether keep-awake turns on at every start
// (config.write_failed on error).
func (s *Service) SetKeepAwakeDefault(on bool) (Snapshot, error) {
	return s.store.SetKeepAwakeDefault(on)
}

// SetAutostart turns start with Windows on or off (autostart.failed on error).
func (s *Service) SetAutostart(on bool) (Snapshot, error) { return s.store.SetAutostart(on) }

// SetTheme sets the theme: "system", "light" or "dark" (config.write_failed on error).
func (s *Service) SetTheme(t config.Theme) (Snapshot, error) { return s.store.SetTheme(t) }

// SetLanguage sets the language: "it" or "en" (config.write_failed on error).
func (s *Service) SetLanguage(l i18n.Lang) (Snapshot, error) { return s.store.SetLanguage(l) }

// Logs returns the most recent log entries, newest first. level is the
// minimum level ("debug", "info", "warn", "error"; "" for all), limit the
// maximum count (0 for the default). logs.read_failed on error.
func (s *Service) Logs(level string, limit int) ([]logging.Entry, error) {
	return logging.Read(s.logsDir, logging.Filter{Level: level, Limit: limit})
}

// PreviewTokens renders a token template (file path, caption, sheet name) as
// it would be right now, in the current language. A malformed template
// returns its token.* error with the 1-based character position in "pos".
func (s *Service) PreviewTokens(template string) (string, error) {
	return tokens.Render(template, s.clock.Now(), s.store.Snapshot().Language)
}

// Preview bounds for PreviewRuns.
const (
	previewDefault = 5
	previewMax     = 50
)

// PreviewRuns returns the next n run times of a cadence from now (RFC 3339,
// local offset), for the editor preview; n 0 means 5, at most 50. An invalid
// cadence returns its cadence.* or schedule.no_days error.
func (s *Service) PreviewRuns(c config.Cadence, n int) ([]string, error) {
	if n <= 0 {
		n = previewDefault
	}
	n = min(n, previewMax)
	runs, err := scheduler.Next(c, s.clock.Now(), n, s.loc)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.Format(time.RFC3339)
	}
	return out, nil
}

// Providers returns the available providers with their capabilities and
// field schemas.
func (s *Service) Providers() []provider.Info { return s.registry.Infos() }

// ListChannels returns every configured channel with its status.
func (s *Service) ListChannels() []provider.ChannelInfo { return s.channels.List() }

// CreateChannel adds a channel of a provider with a name and opens it
// (channel.name_required, channel.provider_unknown, config.write_failed).
// A provider with pairing then needs StartPairing.
func (s *Service) CreateChannel(providerType, name string) (provider.ChannelInfo, error) {
	return s.channels.CreateChannel(providerType, name)
}

// RenameChannel changes a channel's name (channel.name_required,
// channel.not_found, config.write_failed).
func (s *Service) RenameChannel(id, name string) (provider.ChannelInfo, error) {
	return s.channels.RenameChannel(id, name)
}

// ChannelUsage lists the schedules and error recipients that deleting the
// channel affects, for the confirmation dialog.
func (s *Service) ChannelUsage(id string) (provider.Usage, error) { return s.channels.Usage(id) }

// DeleteChannel deletes a channel for good, after the confirmation: it logs
// out where possible, removes its recipients (disabling the schedules left
// without any) and its data. The result says whether the session was removed
// from the service.
func (s *Service) DeleteChannel(id string) (provider.DeleteResult, error) {
	return s.channels.DeleteChannel(id)
}

// StartPairing starts the pairing of a channel (WhatsApp: QR codes through
// EventChannelQR). A paired channel is logged out first (re-pair). Acts for
// real on the account.
func (s *Service) StartPairing(id string) (provider.ChannelInfo, error) {
	return s.channels.StartPairing(id)
}

// CancelPairing stops a pairing in progress.
func (s *Service) CancelPairing(id string) error { return s.channels.CancelPairing(id) }

// Logout removes the channel's session from the service (WhatsApp: the
// linked device) and keeps the channel for a new pairing
// (channel.not_connected, channel.logout_failed). Acts for real.
func (s *Service) Logout(id string) (provider.ChannelInfo, error) { return s.channels.Logout(id) }

// ValidateTarget normalizes and checks a recipient's target on a channel
// (wa.number_invalid, wa.not_on_whatsapp, wa.group_not_found…). When the
// channel is not connected the result is unverified with the
// recipient.check_unavailable warning, and can still be saved.
func (s *Service) ValidateTarget(ctx context.Context, channelID string, target json.RawMessage) (provider.TargetCheck, error) {
	ctx, cancel := context.WithTimeout(ctx, channelCallTimeout)
	defer cancel()
	return s.channels.ValidateTarget(ctx, channelID, target)
}

// ListTargets returns the groups/chats of a connected channel to pick from
// (channel.not_connected, channel.targets_failed).
func (s *Service) ListTargets(ctx context.Context, channelID string) ([]provider.Target, error) {
	ctx, cancel := context.WithTimeout(ctx, channelCallTimeout)
	defer cancel()
	return s.channels.ListTargets(ctx, channelID)
}
