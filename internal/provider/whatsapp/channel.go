package whatsapp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/provider"
)

// keepAliveMaxFail is how long failing keep-alives are tolerated before the
// connection is dropped and retried (whatsmeow does the same only with its
// own reconnect, which is off).
const keepAliveMaxFail = 3 * time.Minute

// channel is one WhatsApp account. It never holds mu while calling the
// client: whatsmeow may wait for the event handler, which takes mu.
type channel struct {
	cli  waClient
	log  *slog.Logger
	clk  clock.Clock
	on   func(provider.Status)
	stop context.CancelFunc
	ctx  context.Context // ends with Close

	mu      sync.Mutex
	status  provider.Status
	closed  bool
	pairing bool
}

func newChannel(cli waClient, deps provider.Deps) *channel {
	if deps.Clock == nil {
		deps.Clock = clock.Real{}
	}
	if deps.Log == nil {
		deps.Log = slog.New(slog.DiscardHandler)
	}
	ctx, stop := context.WithCancel(context.Background())
	c := &channel{cli: cli, log: deps.Log, clk: deps.Clock, on: deps.OnStatus, ctx: ctx, stop: stop}
	if cli.IsPaired() {
		c.status = provider.Status{State: provider.StateDisconnected, Account: maskAccount(cli.OwnNumber())}
	} else {
		c.status = provider.Status{State: provider.StateNotPaired}
	}
	cli.SetHandler(c.onEvent)
	return c
}

func maskAccount(number string) string {
	if number == "" {
		return ""
	}
	return logging.MaskPhone(number)
}

func errRef(err error) *apperr.Ref { return apperr.RefOf(err) }

func errSessionCorrupt() error {
	return apperr.New(apperr.WASessionCorrupt, errors.New("session.db unreadable"))
}

// setStatus changes the status and notifies when it changed.
func (c *channel) setStatus(change func(s *provider.Status)) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	prev := c.status
	change(&c.status)
	next := c.status
	c.mu.Unlock()
	if !sameStatus(prev, next) && c.on != nil {
		c.on(next)
	}
}

func sameStatus(a, b provider.Status) bool {
	if a.State != b.State || a.Account != b.Account || a.Retry != b.Retry || (a.Error == nil) != (b.Error == nil) {
		return false
	}
	return a.Error == nil || a.Error.Code == b.Error.Code
}

func (c *channel) Status() provider.Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *channel) set(state provider.State, retry bool, err error) func(*provider.Status) {
	return func(s *provider.Status) {
		s.State, s.Retry, s.Error = state, retry, errRef(err)
	}
}

// Connect connects a paired session. The client dials and logs in; the
// Connected event then makes the channel connected.
func (c *channel) Connect(context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("channel closed")
	}
	st, pairing := c.status.State, c.pairing
	c.mu.Unlock()
	if pairing || st == provider.StateConnected || st == provider.StateConnecting {
		return nil
	}
	if !c.cli.IsPaired() {
		c.setStatus(func(s *provider.Status) {
			if s.State != provider.StateLoggedOut {
				s.State, s.Retry = provider.StateNotPaired, false
			}
			s.Account = ""
		})
		return nil
	}
	c.setStatus(func(s *provider.Status) {
		s.State, s.Retry, s.Error = provider.StateConnecting, false, nil
		s.Account = maskAccount(c.cli.OwnNumber())
	})
	if err := c.cli.Connect(); err != nil {
		c.setStatus(c.set(provider.StateDisconnected, true, apperr.New(apperr.ChannelDisconnected, err)))
		return fmt.Errorf("connect: %w", err)
	}
	return nil
}

// onEvent maps the client's connection events to the status (see the
// package doc). It must not block.
func (c *channel) onEvent(ev waEvent) {
	switch ev.kind {
	case evConnected:
		c.setStatus(func(s *provider.Status) {
			s.State, s.Retry, s.Error = provider.StateConnected, false, nil
			s.Account = maskAccount(c.cli.OwnNumber())
		})
	case evDisconnected:
		c.setStatus(func(s *provider.Status) {
			switch {
			case s.State == provider.StateLoggedOut || s.State == provider.StateOutdated || s.State == provider.StateNotPaired:
			case s.State == provider.StateDisconnected && !s.Retry && s.Error != nil:
				// A final disconnection (stream replaced) stays final.
			case s.State == provider.StateError && !s.Retry:
			default:
				s.State, s.Retry, s.Error = provider.StateDisconnected, true, errRef(apperr.New(apperr.ChannelDisconnected, errors.New("connection lost")))
			}
		})
	case evConnectFailure:
		c.setStatus(c.set(provider.StateError, true, apperr.New(apperr.ChannelDisconnected, errors.New("connection refused by the server"))))
	case evLoggedOut:
		c.setStatus(func(s *provider.Status) {
			s.State, s.Retry, s.Account = provider.StateLoggedOut, false, ""
			s.Error = errRef(apperr.New(apperr.WALoggedOut, errors.New("logged out from the phone or expired")))
		})
	case evOutdated:
		c.setStatus(c.set(provider.StateOutdated, false, apperr.New(apperr.WAClientOutdated, errors.New("client outdated"))))
		go c.cli.Disconnect()
	case evStreamReplaced:
		c.setStatus(c.set(provider.StateDisconnected, false, apperr.New(apperr.WAStreamReplaced, errors.New("stream replaced"))))
	case evTempBan:
		c.setStatus(c.set(provider.StateError, false, apperr.New(apperr.WATemporaryBan, errors.New("temporary ban"))))
		go c.cli.Disconnect()
	case evPairSuccess:
		c.setStatus(func(s *provider.Status) {
			s.State, s.Retry, s.Error = provider.StateConnecting, false, nil
			s.Account = maskAccount(ev.account)
		})
	case evKeepAliveTimeout:
		if !ev.lastSuccess.IsZero() && c.clk.Now().Sub(ev.lastSuccess) > keepAliveMaxFail {
			c.log.Warn("whatsapp keep-alive failing, reconnecting")
			go func() {
				c.cli.Disconnect()
				c.onEvent(waEvent{kind: evDisconnected})
			}()
		}
	case evKeepAliveRestored:
	}
}

// Close disconnects and closes the session database.
func (c *channel) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	c.stop()
	if err := c.cli.Close(); err != nil {
		return fmt.Errorf("close whatsapp session: %w", err)
	}
	return nil
}

func (c *channel) connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status.State == provider.StateConnected && !c.closed
}

func notConnected() error {
	return apperr.New(apperr.ChannelNotConnected, errors.New("whatsapp not connected"))
}

// StartPairing streams QR codes until the phone links the device, the codes
// run out or ctx ends. See provider.Pairer.
func (c *channel) StartPairing(ctx context.Context) (<-chan provider.PairEvent, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("channel closed")
	}
	if c.pairing {
		c.mu.Unlock()
		return nil, apperr.New(apperr.Internal, errors.New("pairing already in progress"))
	}
	c.pairing = true
	c.mu.Unlock()
	done := func() {
		c.mu.Lock()
		c.pairing = false
		c.mu.Unlock()
	}
	if c.cli.IsPaired() {
		done()
		return nil, apperr.New(apperr.Internal, errors.New("already paired: log out first"))
	}
	// A pre-login socket left over (e.g. an earlier pairing) must go first.
	c.cli.Disconnect()
	qr, err := c.cli.QRChannel(ctx)
	if err != nil {
		done()
		return nil, apperr.New(apperr.WAPairFailed, fmt.Errorf("qr channel: %w", err))
	}
	c.setStatus(c.set(provider.StateNotPaired, false, nil))
	if err := c.cli.Connect(); err != nil {
		done()
		return nil, apperr.New(apperr.WAPairFailed, fmt.Errorf("connect for pairing: %w", err))
	}
	out := make(chan provider.PairEvent, 1)
	go c.followQR(ctx, qr, out, done)
	return out, nil
}

func (c *channel) followQR(ctx context.Context, qr <-chan qrItem, out chan<- provider.PairEvent, done func()) {
	defer close(out)
	defer done()
	send := func(ev provider.PairEvent) bool {
		select {
		case out <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}
	fail := func(err error) {
		c.cli.Disconnect()
		send(provider.PairEvent{Kind: provider.PairFailed, Err: err})
	}
	for {
		var it qrItem
		var ok bool
		select {
		case it, ok = <-qr:
		case <-ctx.Done():
			c.cli.Disconnect()
			return
		}
		if !ok {
			fail(apperr.New(apperr.WAPairFailed, errors.New("qr channel closed")))
			return
		}
		switch it.event {
		case "code":
			if !send(provider.PairEvent{Kind: provider.PairCode, Code: it.code}) {
				c.cli.Disconnect()
				return
			}
		case "success":
			// The client reconnects by itself after pairing; Connected follows.
			send(provider.PairEvent{Kind: provider.PairSuccess})
			return
		case "timeout":
			c.cli.Disconnect()
			send(provider.PairEvent{Kind: provider.PairTimeout, Err: apperr.New(apperr.WAPairTimeout, errors.New("qr codes expired"))})
			return
		case "err-client-outdated":
			c.onEvent(waEvent{kind: evOutdated})
			fail(apperr.New(apperr.WAClientOutdated, errors.New("client outdated during pairing")))
			return
		default:
			cause := it.err
			if cause == nil {
				cause = fmt.Errorf("pairing event %q", it.event)
			}
			fail(apperr.New(apperr.WAPairFailed, cause))
			return
		}
	}
}

// Logout removes the linked device from the phone and deletes the session.
func (c *channel) Logout(ctx context.Context) error {
	if !c.connected() {
		return notConnected()
	}
	if err := c.cli.Logout(ctx); err != nil {
		return apperr.New(apperr.ChannelLogoutFailed, fmt.Errorf("logout: %w", err))
	}
	c.setStatus(func(s *provider.Status) {
		s.State, s.Retry, s.Error, s.Account = provider.StateNotPaired, false, nil, ""
	})
	return nil
}

// ValidateTarget: see provider.Channel and the package doc.
func (c *channel) ValidateTarget(ctx context.Context, raw json.RawMessage) (json.RawMessage, string, error) {
	t, err := parseTarget(raw)
	if err != nil {
		return nil, "", err
	}
	unavailable := func(cause error) (json.RawMessage, string, error) {
		return t.raw(), t.Number, apperr.New(apperr.RecipientCheckUnavailable, cause)
	}
	if !c.connected() {
		return unavailable(errors.New("whatsapp not connected"))
	}
	switch t.Kind {
	case KindNumber:
		ok, err := c.cli.IsOnWhatsApp(ctx, t.Number)
		if err != nil {
			return unavailable(fmt.Errorf("is on whatsapp: %w", err))
		}
		if !ok {
			return nil, "", errNotOnWhatsApp(t.Number)
		}
		return t.raw(), t.Number, nil
	default:
		groups, err := c.cli.JoinedGroups(ctx)
		if err != nil {
			return unavailable(fmt.Errorf("joined groups: %w", err))
		}
		for _, g := range groups {
			if g.jid == t.JID {
				return t.raw(), g.name, nil
			}
		}
		return nil, "", errGroupNotFound(t.JID)
	}
}

// ListTargets returns the account's groups, by name.
func (c *channel) ListTargets(ctx context.Context) ([]provider.Target, error) {
	if !c.connected() {
		return nil, notConnected()
	}
	groups, err := c.cli.JoinedGroups(ctx)
	if err != nil {
		return nil, apperr.New(apperr.ChannelTargetsFailed, fmt.Errorf("joined groups: %w", err))
	}
	slices.SortFunc(groups, func(a, b group) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)), cmp.Compare(a.jid, b.jid))
	})
	out := make([]provider.Target, 0, len(groups))
	for _, g := range groups {
		out = append(out, provider.Target{Target: target{Kind: KindGroup, JID: g.jid}.raw(), DisplayName: g.name})
	}
	return out, nil
}

var (
	_ provider.Channel       = (*channel)(nil)
	_ provider.Pairer        = (*channel)(nil)
	_ provider.LogoutChannel = (*channel)(nil)
)
