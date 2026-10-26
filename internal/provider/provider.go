// Package provider is the delivery layer, independent of any service.
//
// A provider is a kind of service ("whatsapp", later "telegram", "ntfy"…); a
// channel is one configured account of a provider (config.Channel, several per
// provider allowed, e.g. two WhatsApp numbers). Schedules and error reports
// address recipients: a channel plus a provider-specific target
// (config.Recipient.Target). The core only sees the interfaces below,
// the capabilities and the field schemas: nothing outside
// internal/provider/<type> branches on a provider type or imports its
// library. Providers are registered in a Registry (main wires the real ones,
// tests use provider/fake).
//
// Each channel gets its own dir channels/<id>/ under the data dir for
// sessions and secrets. Secrets (bot tokens, passwords: no provider has one
// yet) are to be stored DPAPI-encrypted (platform.Protect, current user),
// never in config.json, never logged.
//
// Only the pipeline and notify may call Channel.Send (AGENTS.md invariant).
//
// Status: every channel reports its State with change notifications
// (Deps.OnStatus). connected and connecting are healthy; disconnected and
// error are retried by the Manager with a backoff when Status.Retry is set;
// logged_out, not_paired and outdated (and a disconnection the provider marks
// as final, e.g. WhatsApp's StreamReplaced) wait for the user. outdated means
// the service rejected this client version: the Manager calls its OnOutdated
// hook (the update check, milestone 10).
//
// The Manager (manager.go) owns the lifecycle of every configured channel:
// open, connect, reconnect, pairing, logout, delete, target checks, and the
// recipients' pending verifications (config.Recipient.Verified).
//
// Deleting a channel (after the GUI's confirmation, which lists Usage)
// closes it, logs out where it has a session and is connected (WhatsApp:
// removes the linked device), removes it and its recipients from every
// schedule and from the error recipients (a schedule left with none is
// disabled) and deletes channels/<id>/. Recipients pointing at a channel
// that doesn't exist (hand-edited config) are skipped at run time with
// recipient.channel_missing (pipeline, milestone 6).
//
// Future providers (not now): Telegram (bot token, sendDocument, chat ids),
// ntfy (server URL + topic, optional token, attachments via PUT).
package provider

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
)

// State is a channel's connection state.
type State string

const (
	StateConnected    State = "connected"
	StateConnecting   State = "connecting"
	StateDisconnected State = "disconnected"
	StateLoggedOut    State = "logged_out"
	StateNotPaired    State = "not_paired"
	StateOutdated     State = "outdated"
	StateError        State = "error"
)

// Status is a channel's state with its reason.
type Status struct {
	State State `json:"state"`
	// Error is why the channel is not connected (channel.disconnected,
	// wa.logged_out…); nil when there is nothing to report.
	Error *apperr.Ref `json:"error"`
	// Account is the paired account for display, already masked (WhatsApp:
	// "***567"); "" when not paired.
	Account string `json:"account"`
	// Retry tells the Manager to reconnect by itself (with backoff). It is
	// meaningful only for disconnected and error.
	Retry bool `json:"-"`
}

// Capabilities say what a provider's channels can do; the pipeline checks
// them before each send (send.too_large, send.caption_too_long,
// send.files_unsupported).
type Capabilities struct {
	// Files: the channel can send files (documents).
	Files bool `json:"files"`
	// MaxFileSize in bytes (0: no limit).
	MaxFileSize int64 `json:"max_file_size"`
	// MaxCaptionLen in characters (0: no limit).
	MaxCaptionLen int `json:"max_caption_len"`
	// Targets: the channel has a list of targets to pick from (ListTargets).
	Targets bool `json:"targets"`
	// Pairing: the channel is set up by an interactive pairing (Pairer), not
	// by its settings alone.
	Pairing bool `json:"pairing"`
}

// FieldKind is the input a schema field needs.
type FieldKind string

const (
	FieldText   FieldKind = "text"
	FieldPhone  FieldKind = "phone"
	FieldSelect FieldKind = "select"
	FieldSecret FieldKind = "secret"
	// FieldPick is chosen from ListTargets, not typed.
	FieldPick FieldKind = "pick"
)

// Field describes one field of a provider's settings or target object, for
// the generic forms of the GUI. Labels are i18n keys
// provider.<type>.<schema>.<key> (and .<option> for select options).
type Field struct {
	Key      string    `json:"key"`
	Kind     FieldKind `json:"kind"`
	Required bool      `json:"required"`
	// Options are the values of a select.
	Options []string `json:"options"`
	// When limits the field to targets whose select field has a value, as
	// "kind=number"; "" always applies.
	When string `json:"when"`
}

// Deps are what a channel gets from the app.
type Deps struct {
	Log   *slog.Logger
	Clock clock.Clock
	// AppVersion is the wchron version (x.y.z), e.g. for the device info a
	// service shows to the user.
	AppVersion string
	// OnStatus is called on every status change, from any goroutine; it
	// must not block or call back into the channel.
	OnStatus func(Status)
}

// Provider is a kind of service.
type Provider interface {
	// Type is the provider id stored in config.Channel.Provider.
	Type() string
	Capabilities() Capabilities
	// SettingsSchema describes config.Channel.Settings (non-secret).
	SettingsSchema() []Field
	// TargetSchema describes config.Recipient.Target.
	TargetSchema() []Field
	// Open prepares the channel from its config and data dir (created by the
	// caller) without connecting. It may fail only when the channel can't be
	// used at all; a recoverable problem (e.g. a corrupt session, which then
	// needs a new pairing) is reported through the status instead.
	Open(cfg config.Channel, dir string, deps Deps) (Channel, error)
}

// Target is one entry of a channel's target list (groups, chats).
type Target struct {
	Target      json.RawMessage `json:"target"`
	DisplayName string          `json:"display_name"`
}

// Attachment is a file to send. Path is the processed temp copy; Name is the
// file name the recipient sees.
type Attachment struct {
	Path string
	Name string
	MIME string
}

// Message is what Send delivers: a text (the caption of the first
// attachment, or a plain message when there is none) and the attachments.
type Message struct {
	Text        string
	Attachments []Attachment
}

// Channel is one open account of a provider. All methods are safe for
// concurrent use.
type Channel interface {
	// Connect starts the connection; it returns when the attempt is under
	// way or has failed. A channel that needs pairing first only reports
	// not_paired. The outcome arrives through Deps.OnStatus.
	Connect(ctx context.Context) error
	// Close disconnects and releases every resource (files in the channel
	// dir included); the channel can't be used afterwards.
	Close() error
	Status() Status
	// ValidateTarget normalizes a target and checks it against the service.
	// It returns the normalized target and its display name; when the
	// format is fine but the service can't be asked now, it returns them with
	// a recipient.check_unavailable error.
	ValidateTarget(ctx context.Context, raw json.RawMessage) (json.RawMessage, string, error)
	// ListTargets returns the targets to pick from (channel.not_connected,
	// channel.targets_failed); nil when the provider has no list.
	ListTargets(ctx context.Context) ([]Target, error)
	// Send delivers msg to target. Only the pipeline and notify call it.
	Send(ctx context.Context, target json.RawMessage, msg Message) error
}

// PairKind is the kind of a pairing event.
type PairKind string

const (
	// PairCode carries a new code to show (WhatsApp: QR contents).
	PairCode    PairKind = "code"
	PairSuccess PairKind = "success"
	// PairTimeout: the codes ran out without a scan.
	PairTimeout PairKind = "timeout"
	PairFailed  PairKind = "failed"
)

// PairEvent is one step of a pairing.
type PairEvent struct {
	Kind PairKind
	Code string
	// Err is set for PairTimeout and PairFailed.
	Err error
}

// Pairer is a channel set up by an interactive pairing.
type Pairer interface {
	// StartPairing starts a pairing on an unpaired channel. The events end
	// with a success, timeout or failure, then the channel is closed.
	// Cancelling ctx stops the pairing.
	StartPairing(ctx context.Context) (<-chan PairEvent, error)
}

// LogoutChannel is a channel with a session on the service that can be
// removed (WhatsApp: the linked device). Logout needs the connection
// (channel.not_connected) and leaves the channel unpaired.
type LogoutChannel interface {
	Logout(ctx context.Context) error
}
