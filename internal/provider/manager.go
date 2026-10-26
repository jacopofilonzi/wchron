package provider

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
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
)

// Reconnect backoff: the first retry after BackoffMin, doubling up to
// BackoffMax; reset when the channel connects.
const (
	BackoffMin = 5 * time.Second
	BackoffMax = 5 * time.Minute
)

// Timeouts of the calls the Manager makes on its own.
const (
	logoutTimeout = 20 * time.Second
	verifyTimeout = 30 * time.Second
)

// ErrTargetInvalid marks (with errors.Is, through the apperr chain) a target
// the service says is wrong for good: not on the service, group left. The
// Manager then stores the recipient as not verified. Any other check error
// is treated as temporary.
var ErrTargetInvalid = errors.New("target rejected by the service")

// PairCancelled is the pairing event sent when the user cancels.
const PairCancelled PairKind = "cancelled"

// ConfigStore is the Manager's access to config.json (app.Store).
type ConfigStore interface {
	// Config returns a copy of the current configuration.
	Config() config.Config
	// UpdateConfig applies change to a copy and saves it; on any error
	// (from change or from the save) nothing changes.
	UpdateConfig(change func(*config.Config) error) error
}

// ChannelInfo is a channel as the GUI and the tray see it.
type ChannelInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Status   Status `json:"status"`
	// Pairing is true while a pairing is in progress.
	Pairing bool `json:"pairing"`
}

// PairingEvent is a pairing step for the GUI (event channel:qr).
type PairingEvent struct {
	ChannelID string   `json:"channel_id"`
	Kind      PairKind `json:"kind"`
	// Code is the content of the QR code to show (Kind "code").
	Code  string      `json:"code"`
	Error *apperr.Ref `json:"error"`
}

// TargetCheck is the result of ValidateTarget.
type TargetCheck struct {
	Target      json.RawMessage `json:"target"`
	DisplayName string          `json:"display_name"`
	Verified    config.Verified `json:"verified"`
	// Warning is recipient.check_unavailable when the channel couldn't be
	// asked: the recipient can be saved, unverified.
	Warning *apperr.Ref `json:"warning"`
}

// UsageSchedule is a schedule that loses recipients when a channel goes.
type UsageSchedule struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Recipients is how many of its recipients are on the channel.
	Recipients int `json:"recipients"`
	// Disabled: it has no recipient left, so it will be disabled.
	Disabled bool `json:"disabled"`
}

// Usage lists what deleting a channel affects.
type Usage struct {
	Schedules       []UsageSchedule `json:"schedules"`
	ErrorRecipients int             `json:"error_recipients"`
}

// DeleteResult tells the GUI whether the session on the service was removed.
type DeleteResult struct {
	// WasPaired: the channel had a session (e.g. a WhatsApp linked device).
	WasPaired bool `json:"was_paired"`
	// LoggedOut: that session was removed from the service; when false the
	// user should remove it by hand (WhatsApp: Linked devices on the phone).
	LoggedOut bool `json:"logged_out"`
}

// Options are the Manager's inputs. Registry, Dir and Store are required.
type Options struct {
	Registry *Registry
	// Dir is the channels folder (channels/ under the data dir).
	Dir        string
	Store      ConfigStore
	Log        *slog.Logger
	Clock      clock.Clock
	AppVersion string
	// OnChange gets the whole channel list after every change (Snapshot).
	OnChange func([]ChannelInfo)
	// OnStatus gets the channel that changed (event channel:status).
	OnStatus func(ChannelInfo)
	// OnPairing gets every pairing step (event channel:qr).
	OnPairing func(PairingEvent)
	// OnSchedulesChanged is called after a delete changed the schedules
	// (scheduler.Reload).
	OnSchedulesChanged func()
	// OnOutdated is called when a channel reports outdated (the update
	// check, milestone 10).
	OnOutdated func(channelID string)
	// AfterFunc runs f after d (reconnects); nil is time.AfterFunc. The
	// returned func cancels it.
	AfterFunc func(d time.Duration, f func()) (stop func() bool)
	// NewID returns a new channel id; nil is 16 random hex characters.
	NewID func() string
}

// Manager owns every configured channel. See the package doc.
type Manager struct {
	o Options

	mu       sync.Mutex
	notifyMu sync.Mutex // keeps the callbacks in the order of the changes
	chans    map[string]*managed
	order    []string
	ctx      context.Context
	cancel   context.CancelFunc
	closed   bool
}

type managed struct {
	cfg     config.Channel
	ch      Channel // nil when not open (disabled, open failed)
	gen     int     // bumped on every open, so a closed channel's late statuses are ignored
	status  Status
	backoff time.Duration
	// stopRetry cancels the scheduled reconnect; nil when none.
	stopRetry  func() bool
	cancelPair context.CancelFunc
	pairing    bool
	verifying  bool
}

// NewManager returns a Manager; it opens nothing until Start.
func NewManager(o Options) *Manager {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.Clock == nil {
		o.Clock = clock.Real{}
	}
	if o.AfterFunc == nil {
		o.AfterFunc = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	if o.NewID == nil {
		o.NewID = randomID
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{o: o, chans: map[string]*managed{}, ctx: ctx, cancel: cancel}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	return hex.EncodeToString(b)
}

// Start opens and connects every enabled channel of the configuration. Call
// it once, after the startup backup.
func (m *Manager) Start() {
	cfg := m.o.Store.Config()
	m.mu.Lock()
	for _, c := range cfg.Channels {
		m.chans[c.ID] = &managed{cfg: c, status: Status{State: StateDisconnected}}
		m.order = append(m.order, c.ID)
	}
	var connect []string
	for _, id := range m.order {
		if mc := m.chans[id]; mc.cfg.Enabled && m.openLocked(mc) {
			connect = append(connect, id)
		}
	}
	m.mu.Unlock()
	m.publishAll()
	for _, id := range connect {
		go m.connect(id)
	}
}

// Close disconnects and closes every channel (app shutdown).
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	var chans []Channel
	for _, mc := range m.chans {
		m.stopRetryLocked(mc)
		if mc.ch != nil {
			chans = append(chans, mc.ch)
			mc.ch = nil
		}
	}
	m.mu.Unlock()
	for _, ch := range chans {
		if err := ch.Close(); err != nil {
			m.o.Log.Warn("close channel", "err", err)
		}
	}
}

// openLocked opens mc's channel; on failure its status says why. Called under m.mu.
func (m *Manager) openLocked(mc *managed) bool {
	id := mc.cfg.ID
	p, err := m.o.Registry.Get(mc.cfg.Provider)
	if err != nil {
		m.o.Log.Warn("channel with an unknown provider", "channel", id, "provider", mc.cfg.Provider)
		mc.status = Status{State: StateError, Error: apperr.RefOf(err)}
		return false
	}
	dir := filepath.Join(m.o.Dir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		m.o.Log.Error("create channel dir", "channel", id, "err", err)
		mc.status = Status{State: StateError, Error: apperr.RefOf(apperr.New(apperr.StorageUnavailable, err).With("path", dir))}
		return false
	}
	mc.gen++
	gen := mc.gen
	ch, err := p.Open(mc.cfg, dir, Deps{
		Log:        m.o.Log.With("channel", id, "provider", mc.cfg.Provider),
		Clock:      m.o.Clock,
		AppVersion: m.o.AppVersion,
		OnStatus:   func(s Status) { m.onStatus(id, gen, s) },
	})
	if err != nil {
		m.o.Log.Error("open channel", "channel", id, "err", err)
		mc.status = Status{State: StateError, Error: apperr.RefOf(err)}
		return false
	}
	mc.ch = ch
	mc.status = ch.Status()
	return true
}

// connect makes one connection attempt; a retryable failure schedules the next.
func (m *Manager) connect(id string) {
	m.mu.Lock()
	mc := m.chans[id]
	if mc == nil || mc.ch == nil || m.closed {
		m.mu.Unlock()
		return
	}
	ch := mc.ch
	m.mu.Unlock()
	if err := ch.Connect(m.ctx); err != nil {
		m.o.Log.Warn("connect channel", "channel", id, "err", err)
	}
	m.mu.Lock()
	if mc := m.chans[id]; mc != nil && mc.ch == ch && retryable(mc.status) && !mc.pairing {
		m.scheduleRetryLocked(id, mc)
	}
	m.mu.Unlock()
}

func retryable(s Status) bool {
	return (s.State == StateDisconnected || s.State == StateError) && s.Retry
}

// scheduleRetryLocked plans the next reconnect unless one is planned.
func (m *Manager) scheduleRetryLocked(id string, mc *managed) {
	if mc.stopRetry != nil || m.closed {
		return
	}
	if mc.backoff == 0 {
		mc.backoff = BackoffMin
	} else {
		mc.backoff = min(2*mc.backoff, BackoffMax)
	}
	gen := mc.gen
	m.o.Log.Info("reconnecting later", "channel", id, "in", mc.backoff.String())
	mc.stopRetry = m.o.AfterFunc(mc.backoff, func() {
		m.mu.Lock()
		cur := m.chans[id]
		if cur == nil || cur.gen != gen {
			m.mu.Unlock()
			return
		}
		cur.stopRetry = nil
		ok := retryable(cur.status) && !cur.pairing
		m.mu.Unlock()
		if ok {
			m.connect(id)
		}
	})
}

func (m *Manager) stopRetryLocked(mc *managed) {
	if mc.stopRetry != nil {
		mc.stopRetry()
		mc.stopRetry = nil
	}
}

// onStatus is every channel's Deps.OnStatus.
func (m *Manager) onStatus(id string, gen int, s Status) {
	m.mu.Lock()
	mc := m.chans[id]
	if mc == nil || mc.gen != gen || m.closed {
		m.mu.Unlock()
		return
	}
	prev := mc.status
	mc.status = s
	verify := false
	switch {
	case s.State == StateConnected:
		mc.backoff = 0
		m.stopRetryLocked(mc)
		verify = prev.State != StateConnected && !mc.verifying
		if verify {
			mc.verifying = true
		}
	case retryable(s) && !mc.pairing:
		m.scheduleRetryLocked(id, mc)
	}
	outdated := s.State == StateOutdated && prev.State != StateOutdated
	if s.State != prev.State {
		m.o.Log.Info("channel status", "channel", id, "state", string(s.State), "error", errCode(s.Error))
	}
	m.mu.Unlock()
	if outdated && m.o.OnOutdated != nil {
		m.o.OnOutdated(id)
	}
	m.publish(id)
	if verify {
		go m.verifyPending(id)
	}
}

func errCode(r *apperr.Ref) string {
	if r == nil {
		return ""
	}
	return string(r.Code)
}

// infoLocked builds the GUI view of mc. Status errors get the channel's
// current name, so a rename shows at once.
func infoLocked(mc *managed) ChannelInfo {
	s := mc.status
	if s.Error != nil {
		e := *s.Error
		e.Params = map[string]string{}
		for k, v := range s.Error.Params {
			e.Params[k] = v
		}
		e.Params["channel"] = mc.cfg.Name
		s.Error = &e
	}
	return ChannelInfo{ID: mc.cfg.ID, Provider: mc.cfg.Provider, Name: mc.cfg.Name, Enabled: mc.cfg.Enabled, Status: s, Pairing: mc.pairing}
}

// List returns every configured channel, in config order.
func (m *Manager) List() []ChannelInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked()
}

func (m *Manager) listLocked() []ChannelInfo {
	out := make([]ChannelInfo, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, infoLocked(m.chans[id]))
	}
	return out
}

// publish sends a channel's change (nil id: the list only) to the callbacks.
func (m *Manager) publish(id string) {
	m.notifyMu.Lock()
	defer m.notifyMu.Unlock()
	m.mu.Lock()
	list := m.listLocked()
	var one *ChannelInfo
	if mc := m.chans[id]; mc != nil {
		i := infoLocked(mc)
		one = &i
	}
	m.mu.Unlock()
	if one != nil && m.o.OnStatus != nil {
		m.o.OnStatus(*one)
	}
	if m.o.OnChange != nil {
		m.o.OnChange(list)
	}
}

func (m *Manager) publishAll() { m.publish("") }

func (m *Manager) get(id string) (*managed, error) {
	mc := m.chans[id]
	if mc == nil {
		return nil, apperr.New(apperr.ChannelNotFound, fmt.Errorf("channel %q not found", id)).With("id", id)
	}
	return mc, nil
}

// withChannel sets the channel param of a returned error to its user label.
func withChannel(err error, name string) error {
	if err == nil {
		return nil
	}
	return apperr.As(err).With("channel", name)
}

// CreateChannel adds an enabled channel of a provider, saves it and opens it.
func (m *Manager) CreateChannel(providerType, name string) (ChannelInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ChannelInfo{}, apperr.New(apperr.ChannelNameRequired, nil)
	}
	if _, err := m.o.Registry.Get(providerType); err != nil {
		return ChannelInfo{}, err
	}
	c := config.Channel{ID: m.o.NewID(), Provider: providerType, Name: name, Enabled: true, Settings: json.RawMessage(`{}`)}
	if err := m.o.Store.UpdateConfig(func(cfg *config.Config) error {
		cfg.Channels = append(cfg.Channels, c)
		return nil
	}); err != nil {
		return ChannelInfo{}, err
	}
	m.mu.Lock()
	mc := &managed{cfg: c, status: Status{State: StateDisconnected}}
	m.chans[c.ID] = mc
	m.order = append(m.order, c.ID)
	opened := m.openLocked(mc)
	info := infoLocked(mc)
	m.mu.Unlock()
	m.o.Log.Info("channel created", "channel", c.ID, "provider", providerType)
	m.publish(c.ID)
	if opened {
		go m.connect(c.ID)
	}
	return info, nil
}

// RenameChannel changes a channel's user label.
func (m *Manager) RenameChannel(id, name string) (ChannelInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ChannelInfo{}, apperr.New(apperr.ChannelNameRequired, nil)
	}
	m.mu.Lock()
	_, err := m.get(id)
	m.mu.Unlock()
	if err != nil {
		return ChannelInfo{}, err
	}
	if err := m.o.Store.UpdateConfig(func(cfg *config.Config) error {
		i := slices.IndexFunc(cfg.Channels, func(c config.Channel) bool { return c.ID == id })
		if i < 0 {
			return apperr.New(apperr.ChannelNotFound, fmt.Errorf("channel %q not in config", id)).With("id", id)
		}
		cfg.Channels[i].Name = name
		return nil
	}); err != nil {
		return ChannelInfo{}, err
	}
	m.mu.Lock()
	mc, err := m.get(id)
	if err != nil {
		m.mu.Unlock()
		return ChannelInfo{}, err
	}
	mc.cfg.Name = name
	info := infoLocked(mc)
	m.mu.Unlock()
	m.publish(id)
	return info, nil
}

// StartPairing starts an interactive pairing; its steps arrive through
// OnPairing. A paired channel is logged out first (re-pair). A pairing
// already in progress is replaced.
func (m *Manager) StartPairing(id string) (ChannelInfo, error) {
	m.mu.Lock()
	mc, err := m.get(id)
	if err != nil {
		m.mu.Unlock()
		return ChannelInfo{}, err
	}
	if mc.ch == nil && mc.cfg.Enabled && !m.openLocked(mc) {
		info := infoLocked(mc)
		m.mu.Unlock()
		return info, withChannel(apperr.New(apperr.ChannelNotConnected, errors.New("channel can't be opened")), mc.cfg.Name)
	}
	pairer, ok := mc.ch.(Pairer)
	if !ok {
		m.mu.Unlock()
		return ChannelInfo{}, apperr.New(apperr.Internal, fmt.Errorf("provider %q has no pairing", mc.cfg.Provider))
	}
	ch, name, paired := mc.ch, mc.cfg.Name, mc.status.Account != ""
	if mc.cancelPair != nil {
		mc.cancelPair()
		mc.cancelPair = nil
	}
	m.stopRetryLocked(mc)
	m.mu.Unlock()

	if paired {
		lc, ok := ch.(LogoutChannel)
		if !ok {
			return ChannelInfo{}, apperr.New(apperr.Internal, fmt.Errorf("provider %q can't log out", mc.cfg.Provider))
		}
		ctx, cancel := context.WithTimeout(m.ctx, logoutTimeout)
		err := lc.Logout(ctx)
		cancel()
		if err != nil {
			return ChannelInfo{}, withChannel(logoutError(err), name)
		}
		m.o.Log.Info("logged out before re-pairing", "channel", id)
	}

	ctx, cancel := context.WithCancel(m.ctx)
	events, err := pairer.StartPairing(ctx)
	if err != nil {
		cancel()
		return ChannelInfo{}, withChannel(err, name)
	}
	m.mu.Lock()
	mc.cancelPair = cancel
	mc.pairing = true
	info := infoLocked(mc)
	m.mu.Unlock()
	m.o.Log.Info("pairing started", "channel", id)
	m.publish(id)
	go m.followPairing(id, ctx, cancel, events)
	return info, nil
}

func (m *Manager) followPairing(id string, ctx context.Context, cancel context.CancelFunc, events <-chan PairEvent) {
	defer cancel()
	for {
		var ev PairEvent
		var ok bool
		select {
		case ev, ok = <-events:
		case <-ctx.Done():
			ev, ok = PairEvent{Kind: PairCancelled}, true
		}
		if !ok {
			ev = PairEvent{Kind: PairFailed, Err: apperr.New(apperr.Internal, errors.New("pairing ended without a result"))}
		}
		m.mu.Lock()
		mc := m.chans[id]
		if mc == nil {
			m.mu.Unlock()
			return
		}
		name := mc.cfg.Name
		done := ev.Kind != PairCode
		if done {
			mc.pairing = false
			mc.cancelPair = nil
		}
		m.mu.Unlock()
		pe := PairingEvent{ChannelID: id, Kind: ev.Kind, Code: ev.Code}
		if ev.Err != nil {
			pe.Error = apperr.RefOf(withChannel(ev.Err, name))
		}
		if done {
			m.o.Log.Info("pairing ended", "channel", id, "result", string(ev.Kind), "error", errCode(pe.Error))
		}
		m.notifyMu.Lock()
		if m.o.OnPairing != nil {
			m.o.OnPairing(pe)
		}
		m.notifyMu.Unlock()
		if done {
			m.publish(id)
			return
		}
	}
}

// CancelPairing stops a pairing in progress (no error when there is none).
func (m *Manager) CancelPairing(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc, err := m.get(id)
	if err != nil {
		return err
	}
	if mc.cancelPair != nil {
		mc.cancelPair()
	}
	return nil
}

func logoutError(err error) error {
	if e := apperr.As(err); e.Code != apperr.Internal {
		return e
	}
	return apperr.New(apperr.ChannelLogoutFailed, err)
}

// Logout removes the channel's session from the service (WhatsApp: the
// linked device) and keeps the channel, ready for a new pairing.
func (m *Manager) Logout(id string) (ChannelInfo, error) {
	m.mu.Lock()
	mc, err := m.get(id)
	if err != nil {
		m.mu.Unlock()
		return ChannelInfo{}, err
	}
	lc, ok := mc.ch.(LogoutChannel)
	if !ok {
		m.mu.Unlock()
		return ChannelInfo{}, apperr.New(apperr.Internal, fmt.Errorf("provider %q can't log out", mc.cfg.Provider))
	}
	name := mc.cfg.Name
	if mc.status.State != StateConnected {
		m.mu.Unlock()
		return ChannelInfo{}, withChannel(apperr.New(apperr.ChannelNotConnected, errors.New("logout needs the connection")), name)
	}
	m.stopRetryLocked(mc)
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(m.ctx, logoutTimeout)
	defer cancel()
	if err := lc.Logout(ctx); err != nil {
		m.o.Log.Warn("logout", "channel", id, "err", err)
		return ChannelInfo{}, withChannel(logoutError(err), name)
	}
	m.o.Log.Info("logged out", "channel", id)
	m.mu.Lock()
	info := infoLocked(mc)
	m.mu.Unlock()
	m.publish(id)
	return info, nil
}

// Usage lists the schedules and error recipients that use a channel.
func (m *Manager) Usage(id string) (Usage, error) {
	m.mu.Lock()
	_, err := m.get(id)
	m.mu.Unlock()
	if err != nil {
		return Usage{}, err
	}
	return channelUsage(m.o.Store.Config(), id), nil
}

func channelUsage(cfg config.Config, id string) Usage {
	u := Usage{Schedules: []UsageSchedule{}}
	for _, s := range cfg.Schedules {
		n := 0
		for _, r := range s.Recipients {
			if r.ChannelID == id {
				n++
			}
		}
		if n > 0 {
			u.Schedules = append(u.Schedules, UsageSchedule{ID: s.ID, Name: s.Name, Recipients: n, Disabled: n == len(s.Recipients)})
		}
	}
	for _, r := range cfg.Settings.ErrorRecipients {
		if r.ChannelID == id {
			u.ErrorRecipients++
		}
	}
	return u
}

// removeChannel drops a channel and its recipients from cfg; a schedule
// left without recipients is disabled.
func removeChannel(cfg *config.Config, id string) {
	cfg.Channels = slices.DeleteFunc(cfg.Channels, func(c config.Channel) bool { return c.ID == id })
	onChannel := func(r config.Recipient) bool { return r.ChannelID == id }
	cfg.Settings.ErrorRecipients = slices.DeleteFunc(cfg.Settings.ErrorRecipients, onChannel)
	for i := range cfg.Schedules {
		s := &cfg.Schedules[i]
		before := len(s.Recipients)
		s.Recipients = slices.DeleteFunc(s.Recipients, onChannel)
		if before > 0 && len(s.Recipients) == 0 {
			s.Enabled = false
		}
	}
}

// DeleteChannel deletes a channel for good: it logs out where the channel
// has a session and is connected (best effort), closes it, removes it and
// its recipients from the configuration (disabling the schedules left
// without recipients), then deletes its data dir.
func (m *Manager) DeleteChannel(id string) (DeleteResult, error) {
	m.mu.Lock()
	mc, err := m.get(id)
	if err != nil {
		m.mu.Unlock()
		return DeleteResult{}, err
	}
	if mc.cancelPair != nil {
		mc.cancelPair()
		mc.cancelPair = nil
	}
	m.stopRetryLocked(mc)
	ch := mc.ch
	res := DeleteResult{WasPaired: mc.status.Account != ""}
	connected := mc.status.State == StateConnected
	mc.ch = nil
	mc.gen++ // late statuses of the closed channel are ignored
	m.mu.Unlock()

	if lc, ok := ch.(LogoutChannel); ok && res.WasPaired && connected {
		ctx, cancel := context.WithTimeout(m.ctx, logoutTimeout)
		if err := lc.Logout(ctx); err != nil {
			m.o.Log.Warn("logout before delete failed; the session stays on the service", "channel", id, "err", err)
		} else {
			res.LoggedOut = true
		}
		cancel()
	}
	if ch != nil {
		if err := ch.Close(); err != nil {
			m.o.Log.Warn("close channel before delete", "channel", id, "err", err)
		}
	}

	if err := m.o.Store.UpdateConfig(func(cfg *config.Config) error {
		removeChannel(cfg, id)
		return nil
	}); err != nil {
		// Still configured: open it again so it keeps working.
		m.mu.Lock()
		opened := !m.closed && mc.cfg.Enabled && m.openLocked(mc)
		m.mu.Unlock()
		m.publish(id)
		if opened {
			go m.connect(id)
		}
		return DeleteResult{}, err
	}

	m.mu.Lock()
	delete(m.chans, id)
	m.order = slices.DeleteFunc(m.order, func(s string) bool { return s == id })
	m.mu.Unlock()
	if err := os.RemoveAll(filepath.Join(m.o.Dir, id)); err != nil {
		m.o.Log.Error("delete channel dir", "channel", id, "err", err)
	}
	m.o.Log.Info("channel deleted", "channel", id, "logged_out", res.LoggedOut)
	m.publishAll()
	if m.o.OnSchedulesChanged != nil {
		m.o.OnSchedulesChanged()
	}
	return res, nil
}

// ValidateTarget normalizes and checks a target on a channel. When the
// channel can't be asked now, the result is unverified with the
// recipient.check_unavailable warning: the recipient can still be saved.
func (m *Manager) ValidateTarget(ctx context.Context, channelID string, raw json.RawMessage) (TargetCheck, error) {
	m.mu.Lock()
	mc, err := m.get(channelID)
	if err != nil {
		m.mu.Unlock()
		return TargetCheck{}, err
	}
	ch, name := mc.ch, mc.cfg.Name
	m.mu.Unlock()
	if ch == nil {
		return TargetCheck{}, withChannel(apperr.New(apperr.ChannelNotConnected, errors.New("channel not open")), name)
	}
	norm, display, err := ch.ValidateTarget(ctx, raw)
	if err != nil {
		e := apperr.As(withChannel(err, name))
		if e.Code == apperr.RecipientCheckUnavailable && norm != nil {
			return TargetCheck{Target: norm, DisplayName: display, Verified: config.VerifiedUnknown, Warning: apperr.RefOf(e)}, nil
		}
		return TargetCheck{}, e
	}
	return TargetCheck{Target: norm, DisplayName: display, Verified: config.VerifiedYes}, nil
}

// ListTargets returns a channel's targets to pick from.
func (m *Manager) ListTargets(ctx context.Context, channelID string) ([]Target, error) {
	m.mu.Lock()
	mc, err := m.get(channelID)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	ch, name := mc.ch, mc.cfg.Name
	m.mu.Unlock()
	if ch == nil {
		return nil, withChannel(apperr.New(apperr.ChannelNotConnected, errors.New("channel not open")), name)
	}
	ts, err := ch.ListTargets(ctx)
	if err != nil {
		return nil, withChannel(err, name)
	}
	if ts == nil {
		ts = []Target{}
	}
	return ts, nil
}

// verifyPending checks the unverified recipients of a channel that just
// connected and stores the results. A rejected target becomes "no" (never
// sent to) and is logged as recipient.check_failed; a temporary failure
// leaves it unverified for the next connection.
func (m *Manager) verifyPending(id string) {
	defer func() {
		m.mu.Lock()
		if mc := m.chans[id]; mc != nil {
			mc.verifying = false
		}
		m.mu.Unlock()
	}()
	cfg := m.o.Store.Config()
	type result struct {
		verified config.Verified
		target   json.RawMessage
		display  string
	}
	// Recipient ids are unique only within a list: results go by target.
	results := map[string]result{}
	var pending []config.Recipient
	collect := func(rs []config.Recipient) {
		for _, r := range rs {
			if r.ChannelID == id && r.Verified == config.VerifiedUnknown {
				pending = append(pending, r)
			}
		}
	}
	collect(cfg.Settings.ErrorRecipients)
	for _, s := range cfg.Schedules {
		collect(s.Recipients)
	}
	if len(pending) == 0 {
		return
	}
	m.o.Log.Info("checking unverified recipients", "channel", id, "count", len(pending))
	tried := map[string]bool{}
	for _, r := range pending {
		key := string(r.Target)
		if tried[key] {
			continue
		}
		tried[key] = true
		ctx, cancel := context.WithTimeout(m.ctx, verifyTimeout)
		chk, err := m.ValidateTarget(ctx, id, r.Target)
		cancel()
		switch {
		case err == nil && chk.Warning == nil:
			results[key] = result{verified: config.VerifiedYes, target: chk.Target, display: chk.DisplayName}
		case err != nil && errors.Is(err, ErrTargetInvalid):
			results[key] = result{verified: config.VerifiedNo}
			m.o.Log.Warn("recipient failed the check: it won't receive anything", "code", string(apperr.RecipientCheckFailed),
				"channel", id, "recipient", r.ID, "reason", string(apperr.As(err).Code))
		default:
			// Disconnected again or a temporary failure: try at the next connection.
			m.o.Log.Info("recipient check postponed", "channel", id, "recipient", r.ID, "err", err)
		}
	}
	if len(results) == 0 {
		return
	}
	apply := func(rs []config.Recipient) {
		for i := range rs {
			res, ok := results[string(rs[i].Target)]
			if !ok || rs[i].ChannelID != id || rs[i].Verified != config.VerifiedUnknown {
				continue
			}
			rs[i].Verified = res.verified
			if res.target != nil {
				rs[i].Target = res.target
			}
			if res.display != "" {
				rs[i].DisplayName = res.display
			}
		}
	}
	if err := m.o.Store.UpdateConfig(func(c *config.Config) error {
		apply(c.Settings.ErrorRecipients)
		for i := range c.Schedules {
			apply(c.Schedules[i].Recipients)
		}
		return nil
	}); err != nil {
		m.o.Log.Error("save recipient checks", "channel", id, "err", err)
	}
}
