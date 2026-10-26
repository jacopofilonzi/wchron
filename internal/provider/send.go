package provider

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
)

// Pause between two consecutive sends on the same channel (any schedule,
// error reports included), so the account doesn't look like a bot burst.
const (
	SendPauseMin = 2 * time.Second
	SendPauseMax = 5 * time.Second
)

func randomPause() time.Duration {
	return SendPauseMin + rand.N(SendPauseMax-SendPauseMin+1)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// lane serializes the sends of one channel: one at a time, and the next one
// starts at least a pause after the previous one ended. The slot is a
// channel (not a mutex) so waiting for it can be cancelled.
type lane struct {
	slot    chan struct{}
	lastEnd time.Time
}

func (m *Manager) laneFor(id string) *lane {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.lanes[id]
	if l == nil {
		l = &lane{slot: make(chan struct{}, 1)}
		m.lanes[id] = l
	}
	return l
}

// SendInfo is what the pipeline checks before a send.
type SendInfo struct {
	Name         string
	Connected    bool
	Capabilities Capabilities
}

// SendInfo returns a configured channel's name, connection and
// capabilities; ok is false when the channel doesn't exist.
func (m *Manager) SendInfo(id string) (SendInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc := m.chans[id]
	if mc == nil {
		return SendInfo{}, false
	}
	info := SendInfo{Name: mc.cfg.Name, Connected: mc.ch != nil && mc.status.State == StateConnected}
	if p, err := m.o.Registry.Get(mc.cfg.Provider); err == nil {
		info.Capabilities = p.Capabilities()
	}
	return info, true
}

// Send delivers msg to target on a channel. Sends on the same channel are
// serialized across every caller (the runs of all schedules and the error
// reports) with a random 2–5 s pause between them; different channels send in
// parallel. A cancelled ctx stops the wait. Only the pipeline and notify call
// it (AGENTS.md invariant). Errors: channel.not_found, send.not_connected, or
// the provider's send.* error.
func (m *Manager) Send(ctx context.Context, id string, target json.RawMessage, msg Message) error {
	l := m.laneFor(id)
	select {
	case l.slot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-l.slot }()
	if !l.lastEnd.IsZero() {
		wait := l.lastEnd.Add(m.o.SendPause()).Sub(m.o.Clock.Now())
		if err := m.o.Sleep(ctx, wait); err != nil {
			return err
		}
	}
	m.mu.Lock()
	mc, err := m.get(id)
	var ch Channel
	if err == nil {
		ch = mc.ch
	}
	m.mu.Unlock()
	if err != nil {
		return err
	}
	if ch == nil {
		return apperr.New(apperr.SendNotConnected, errors.New("channel not open"))
	}
	err = ch.Send(ctx, target, msg)
	l.lastEnd = m.o.Clock.Now()
	return err
}

// VerifyRecipient checks an unverified target right before a send (the
// channel must be connected) and stores the outcome on every recipient with
// that target on the channel, like the check at connection: "yes" (with the
// normalized target and display name) or "no" (ErrTargetInvalid). A
// temporary failure returns VerifiedUnknown with the error and stores nothing.
func (m *Manager) VerifyRecipient(ctx context.Context, id string, target json.RawMessage) (config.Verified, json.RawMessage, error) {
	chk, err := m.ValidateTarget(ctx, id, target)
	var res checkResult
	switch {
	case err == nil && chk.Warning == nil:
		res = checkResult{verified: config.VerifiedYes, target: chk.Target, display: chk.DisplayName}
	case err != nil && errors.Is(err, ErrTargetInvalid):
		res = checkResult{verified: config.VerifiedNo}
		m.o.Log.Warn("recipient failed the check before a send: it won't receive anything", "code", string(apperr.RecipientCheckFailed),
			"channel", id, "reason", string(apperr.As(err).Code))
	default:
		if err == nil {
			err = apperr.New(chk.Warning.Code, errors.New("check unavailable"))
		}
		return config.VerifiedUnknown, target, err
	}
	m.storeChecks(id, map[string]checkResult{string(target): res})
	if res.target == nil {
		res.target = target
	}
	return res.verified, res.target, nil
}

type checkResult struct {
	verified config.Verified
	target   json.RawMessage
	display  string
}

// storeChecks applies check results (by original target) to the unverified
// recipients of a channel in every schedule and in the error recipients.
func (m *Manager) storeChecks(id string, results map[string]checkResult) {
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
