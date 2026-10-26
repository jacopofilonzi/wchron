// Package fake is an in-memory provider for tests (manager, pipeline,
// notify, GUI): nothing leaves the process. Every opened channel is kept in
// Provider.Channels so a test can script it (status changes, failures,
// pairing steps) and inspect what was sent.
//
// Defaults: a channel opens paired (status disconnected) unless
// Provider.Unpaired; Connect connects at once; ValidateTarget accepts any
// target while connected (display name "fake") and returns
// recipient.check_unavailable otherwise; StartPairing emits one code then
// success, and the channel connects; Logout needs the connection.
package fake

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/provider"
)

// Type is the provider type of the fake.
const Type = "fake"

// Account is the masked account a paired fake channel shows.
const Account = "***001"

// Provider is the fake provider.
type Provider struct {
	mu sync.Mutex
	// Caps are returned by Capabilities.
	Caps provider.Capabilities
	// Unpaired makes new channels open not paired.
	Unpaired bool
	// OpenErr makes Open fail.
	OpenErr error
	// OnOpen, when set, scripts every new channel before it is returned.
	OnOpen func(*Channel)
	// Channels are the opened channels by id (the latest open wins).
	Channels map[string]*Channel
}

// New returns a fake provider that can send files of up to 1 MB.
func New() *Provider {
	return &Provider{
		Caps:     provider.Capabilities{Files: true, MaxFileSize: 1 << 20, MaxCaptionLen: 1024, Targets: true, Pairing: true},
		Channels: map[string]*Channel{},
	}
}

func (p *Provider) Type() string                        { return Type }
func (p *Provider) Capabilities() provider.Capabilities { return p.Caps }
func (p *Provider) SettingsSchema() []provider.Field    { return []provider.Field{} }
func (p *Provider) TargetSchema() []provider.Field {
	return []provider.Field{{Key: "to", Kind: provider.FieldText, Required: true}}
}

// Open returns a new fake channel.
func (p *Provider) Open(cfg config.Channel, dir string, deps provider.Deps) (provider.Channel, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.OpenErr != nil {
		return nil, p.OpenErr
	}
	c := &Channel{ID: cfg.ID, Dir: dir, deps: deps, paired: !p.Unpaired}
	if c.paired {
		c.status = provider.Status{State: provider.StateDisconnected, Account: Account}
	} else {
		c.status = provider.Status{State: provider.StateNotPaired}
	}
	if p.OnOpen != nil {
		p.OnOpen(c)
	}
	p.Channels[cfg.ID] = c
	return c, nil
}

// Channel returns the latest opened channel with that id (nil if none).
func (p *Provider) Channel(id string) *Channel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Channels[id]
}

// Sent is one recorded Send.
type Sent struct {
	Target json.RawMessage
	Msg    provider.Message
}

// Channel is a fake channel.
type Channel struct {
	ID  string
	Dir string

	mu     sync.Mutex
	deps   provider.Deps
	status provider.Status
	paired bool
	closed bool

	// ConnectErr makes Connect fail (status disconnected, retryable).
	ConnectErr error
	// Validate replaces the default target check while connected.
	Validate func(raw json.RawMessage) (json.RawMessage, string, error)
	// Targets is what ListTargets returns.
	Targets []provider.Target
	// SendErr makes Send fail after recording.
	SendErr error
	// OnSend, when set, runs after recording and outside the lock (to block,
	// observe concurrency or fail one send); its error replaces SendErr.
	OnSend func(target json.RawMessage, msg provider.Message) error
	// PairEvents replaces the default pairing steps; a nil entry blocks
	// until the pairing is cancelled.
	PairEvents []*provider.PairEvent
	// LogoutErr makes Logout fail.
	LogoutErr error

	Sends    []Sent
	Connects int
	Logouts  int
	Closed   bool
}

// Edit runs f with the channel locked, to change its script fields or read
// its records safely.
func (c *Channel) Edit(f func(c *Channel)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f(c)
}

// SetStatus changes the status and notifies, as a provider event would.
func (c *Channel) SetStatus(s provider.Status) {
	c.mu.Lock()
	c.status = s
	on := c.deps.OnStatus
	c.mu.Unlock()
	if on != nil {
		on(s)
	}
}

func (c *Channel) Status() provider.Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *Channel) Connect(context.Context) error {
	c.mu.Lock()
	c.Connects++
	paired, err := c.paired, c.ConnectErr
	c.mu.Unlock()
	switch {
	case !paired:
		c.SetStatus(provider.Status{State: provider.StateNotPaired})
	case err != nil:
		c.SetStatus(provider.Status{State: provider.StateDisconnected, Account: Account, Retry: true,
			Error: apperr.RefOf(apperr.New(apperr.ChannelDisconnected, err))})
		return err
	default:
		c.SetStatus(provider.Status{State: provider.StateConnected, Account: Account})
	}
	return nil
}

func (c *Channel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed, c.Closed = true, true
	return nil
}

func (c *Channel) ValidateTarget(_ context.Context, raw json.RawMessage) (json.RawMessage, string, error) {
	c.mu.Lock()
	connected, validate := c.status.State == provider.StateConnected, c.Validate
	c.mu.Unlock()
	if !connected {
		return raw, "", apperr.New(apperr.RecipientCheckUnavailable, errors.New("not connected"))
	}
	if validate != nil {
		return validate(raw)
	}
	return raw, "fake", nil
}

func (c *Channel) ListTargets(context.Context) ([]provider.Target, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.State != provider.StateConnected {
		return nil, apperr.New(apperr.ChannelNotConnected, errors.New("not connected"))
	}
	return c.Targets, nil
}

func (c *Channel) Send(_ context.Context, target json.RawMessage, msg provider.Message) error {
	c.mu.Lock()
	c.Sends = append(c.Sends, Sent{Target: target, Msg: msg})
	err, on := c.SendErr, c.OnSend
	c.mu.Unlock()
	if on != nil {
		return on(target, msg)
	}
	return err
}

// StartPairing plays PairEvents (default: a code, then success).
func (c *Channel) StartPairing(ctx context.Context) (<-chan provider.PairEvent, error) {
	c.mu.Lock()
	steps := c.PairEvents
	c.mu.Unlock()
	if steps == nil {
		steps = []*provider.PairEvent{{Kind: provider.PairCode, Code: "fake-qr-1"}, {Kind: provider.PairSuccess}}
	}
	out := make(chan provider.PairEvent)
	go func() {
		defer close(out)
		for _, st := range steps {
			if st == nil {
				<-ctx.Done()
				return
			}
			select {
			case out <- *st:
			case <-ctx.Done():
				return
			}
			if st.Kind == provider.PairSuccess {
				c.mu.Lock()
				c.paired = true
				c.mu.Unlock()
				c.SetStatus(provider.Status{State: provider.StateConnected, Account: Account})
			}
		}
	}()
	return out, nil
}

func (c *Channel) Logout(context.Context) error {
	c.mu.Lock()
	if c.status.State != provider.StateConnected {
		c.mu.Unlock()
		return apperr.New(apperr.ChannelNotConnected, errors.New("not connected"))
	}
	if c.LogoutErr != nil {
		err := c.LogoutErr
		c.mu.Unlock()
		return err
	}
	c.Logouts++
	c.paired = false
	c.mu.Unlock()
	c.SetStatus(provider.Status{State: provider.StateNotPaired})
	return nil
}

// SentMessages returns a copy of the recorded sends.
func (c *Channel) SentMessages() []Sent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Sent(nil), c.Sends...)
}

var (
	_ provider.Provider      = (*Provider)(nil)
	_ provider.Pairer        = (*Channel)(nil)
	_ provider.LogoutChannel = (*Channel)(nil)
)
