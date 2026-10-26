package whatsapp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/provider"
)

// Type is the provider type in config.Channel.Provider.
const Type = "whatsapp"

// DeviceName is how the linked device shows on the phone.
const DeviceName = "wchron"

// Limits. MaxFileSize is a conservative app limit (see the package doc).
const (
	MaxFileSize   = 100 << 20
	MaxCaptionLen = 1024
)

// Provider is the WhatsApp provider.
type Provider struct{}

// New returns the provider.
func New() *Provider { return &Provider{} }

func (*Provider) Type() string { return Type }

func (*Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Files: true, MaxFileSize: MaxFileSize, MaxCaptionLen: MaxCaptionLen, Targets: true, Pairing: true}
}

// SettingsSchema is empty: a WhatsApp channel is set up by pairing alone.
func (*Provider) SettingsSchema() []provider.Field { return []provider.Field{} }

// TargetSchema: a number typed with its international prefix, or a group
// picked from the account's groups.
func (*Provider) TargetSchema() []provider.Field {
	return []provider.Field{
		{Key: "kind", Kind: provider.FieldSelect, Required: true, Options: []string{KindNumber, KindGroup}},
		{Key: "number", Kind: provider.FieldPhone, Required: true, When: "kind=" + KindNumber},
		{Key: "jid", Kind: provider.FieldPick, Required: true, When: "kind=" + KindGroup},
	}
}

// Open opens the channel's session (dir/session.db) without connecting.
func (*Provider) Open(cfg config.Channel, dir string, deps provider.Deps) (provider.Channel, error) {
	if deps.Log == nil {
		deps.Log = slog.New(slog.DiscardHandler)
	}
	setDeviceInfo(deps.AppVersion)
	cli, corrupt, err := openSession(context.Background(), dir, deps.Clock, newLogger(deps.Log))
	if err != nil {
		return nil, fmt.Errorf("open whatsapp session: %w", err)
	}
	c := newChannel(cli, deps)
	if corrupt {
		deps.Log.Error("whatsapp session unreadable: set aside, a new pairing is needed", "code", "wa.session_corrupt")
		c.status.Error = errRef(errSessionCorrupt())
	}
	return c, nil
}

var _ provider.Provider = (*Provider)(nil)
