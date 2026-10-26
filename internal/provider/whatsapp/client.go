package whatsapp

import (
	"context"
	"time"
)

// waClient is what the channel needs from whatsmeow; real.go implements it
// over *whatsmeow.Client, the tests with a fake. Implementations must not
// call the event handler while holding a lock the caller may wait on.
type waClient interface {
	// SetHandler sets the receiver of the connection events (one at a time).
	SetHandler(func(waEvent))
	// IsPaired: the session holds a device that is not logged out.
	IsPaired() bool
	// OwnNumber is the paired account's phone number, digits only ("" if none).
	OwnNumber() string
	// Connect dials and logs in (or starts the pre-login socket for pairing).
	Connect() error
	Disconnect()
	// Logout removes the linked device from the account and deletes the
	// local session.
	Logout(ctx context.Context) error
	// QRChannel prepares a pairing (a fresh device if the old one was logged
	// out); Connect must follow.
	QRChannel(ctx context.Context) (<-chan qrItem, error)
	IsOnWhatsApp(ctx context.Context, number string) (bool, error)
	JoinedGroups(ctx context.Context) ([]group, error)
	// SendDocument uploads data and sends it as a document to jid.
	SendDocument(ctx context.Context, jid string, data []byte, doc document) error
	SendText(ctx context.Context, jid, text string) error
	// Close disconnects and closes the session database.
	Close() error
}

type evKind int

const (
	evConnected evKind = iota + 1
	evDisconnected
	evConnectFailure
	evLoggedOut
	evOutdated
	evStreamReplaced
	evTempBan
	evPairSuccess
	evKeepAliveTimeout
	evKeepAliveRestored
)

// waEvent is a whatsmeow connection event, translated.
type waEvent struct {
	kind evKind
	// lastSuccess is the last good keep-alive (evKeepAliveTimeout).
	lastSuccess time.Time
	// account is the paired number, digits only (evPairSuccess).
	account string
}

// qrItem is a step of whatsmeow's QR channel: event "code" (with code),
// "success", "timeout", "err-client-outdated", or another error.
type qrItem struct {
	event string
	code  string
	err   error
}

type group struct {
	jid  string
	name string
}

type document struct {
	fileName string
	mime     string
	caption  string
}

// sendError says which step of a send failed and how to treat it.
type sendError struct {
	upload bool
	// network: worth one short retry.
	network bool
	// recipient: the service rejected the target (not in the group…).
	recipient bool
	err       error
}

func (e *sendError) Error() string {
	step := "send"
	if e.upload {
		step = "upload"
	}
	return step + ": " + e.err.Error()
}

func (e *sendError) Unwrap() error { return e.err }
