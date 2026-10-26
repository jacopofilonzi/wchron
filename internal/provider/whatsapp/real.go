package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"go.mau.fi/util/exhttp"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var deviceInfoOnce sync.Once

// setDeviceInfo sets what the phone shows for the linked device (whatsmeow
// keeps it in package globals, read at pairing): the name "wchron", the app
// version and a desktop icon.
func setDeviceInfo(appVersion string) {
	deviceInfoOnce.Do(func() {
		var v [3]uint32
		for i, p := range strings.SplitN(appVersion, ".", 3) {
			n, err := strconv.ParseUint(p, 10, 32)
			if err != nil {
				break
			}
			v[i] = uint32(n)
		}
		store.SetOSInfo(DeviceName, v)
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_DESKTOP.Enum()
		store.DeviceProps.RequireFullSync = proto.Bool(false)
	})
}

// realClient is waClient over whatsmeow. The client is replaced by a fresh
// one (new device) when a pairing starts after a logout.
type realClient struct {
	container *sqlstore.Container
	log       waLog.Logger

	mu      sync.Mutex
	cli     *whatsmeow.Client
	handler func(waEvent)
}

func newRealClient(container *sqlstore.Container, dev *store.Device, log waLog.Logger) *realClient {
	rc := &realClient{container: container, log: log}
	rc.cli = rc.newWhatsmeow(dev)
	return rc
}

func (rc *realClient) newWhatsmeow(dev *store.Device) *whatsmeow.Client {
	cli := whatsmeow.NewClient(dev, rc.log.Sub("Client"))
	// The provider.Manager reconnects with its own backoff.
	cli.EnableAutoReconnect = false
	// wchron never reads messages: don't download the history the phone offers.
	cli.ManualHistorySyncDownload = true
	cli.AddEventHandler(rc.dispatch)
	return cli
}

func (rc *realClient) client() *whatsmeow.Client {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.cli
}

func (rc *realClient) SetHandler(h func(waEvent)) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.handler = h
}

// dispatch translates the events the channel cares about. whatsmeow calls it
// synchronously from its own goroutines.
func (rc *realClient) dispatch(evt any) {
	var ev waEvent
	switch v := evt.(type) {
	case *events.Connected:
		ev.kind = evConnected
	case *events.Disconnected:
		ev.kind = evDisconnected
	case *events.ConnectFailure, *events.CATRefreshError:
		ev.kind = evConnectFailure
	case *events.LoggedOut:
		ev.kind = evLoggedOut
	case *events.ClientOutdated:
		ev.kind = evOutdated
	case *events.StreamReplaced:
		ev.kind = evStreamReplaced
	case *events.TemporaryBan:
		ev.kind = evTempBan
	case *events.PairSuccess:
		ev.kind, ev.account = evPairSuccess, v.ID.User
	case *events.KeepAliveTimeout:
		ev.kind, ev.lastSuccess = evKeepAliveTimeout, v.LastSuccess
	case *events.KeepAliveRestored:
		ev.kind = evKeepAliveRestored
	default:
		return
	}
	rc.mu.Lock()
	h := rc.handler
	rc.mu.Unlock()
	if h != nil {
		h(ev)
	}
}

func (rc *realClient) IsPaired() bool {
	cli := rc.client()
	return cli.Store.ID != nil && !cli.Store.Deleted
}

func (rc *realClient) OwnNumber() string {
	cli := rc.client()
	if cli.Store.ID == nil || cli.Store.Deleted {
		return ""
	}
	return cli.Store.ID.User
}

func (rc *realClient) Connect() error {
	err := rc.client().Connect()
	if errors.Is(err, whatsmeow.ErrAlreadyConnected) {
		return nil
	}
	return err
}

func (rc *realClient) Disconnect() { rc.client().Disconnect() }

func (rc *realClient) Logout(ctx context.Context) error { return rc.client().Logout(ctx) }

func (rc *realClient) QRChannel(ctx context.Context) (<-chan qrItem, error) {
	rc.mu.Lock()
	if rc.cli.Store.Deleted || rc.cli.Store.ID != nil {
		// Logged out (the old device row is gone): pair as a new device.
		old := rc.cli
		rc.cli = rc.newWhatsmeow(rc.container.NewDevice())
		rc.mu.Unlock()
		old.Disconnect()
		old.RemoveEventHandlers()
	} else {
		rc.mu.Unlock()
	}
	in, err := rc.client().GetQRChannel(ctx)
	if err != nil {
		return nil, err
	}
	out := make(chan qrItem, 8)
	go func() {
		defer close(out)
		for it := range in {
			out <- qrItem{event: it.Event, code: it.Code, err: it.Error}
		}
	}()
	return out, nil
}

func (rc *realClient) IsOnWhatsApp(ctx context.Context, number string) (bool, error) {
	res, err := rc.client().IsOnWhatsApp(ctx, []string{number})
	if err != nil {
		return false, err
	}
	for _, r := range res {
		if r.IsIn {
			return true, nil
		}
	}
	return false, nil
}

func (rc *realClient) JoinedGroups(ctx context.Context) ([]group, error) {
	gs, err := rc.client().GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]group, 0, len(gs))
	for _, g := range gs {
		if g == nil {
			continue
		}
		out = append(out, group{jid: g.JID.String(), name: g.Name})
	}
	return out, nil
}

func classify(upload bool, err error) error {
	return &sendError{
		upload:  upload,
		network: exhttp.IsNetworkError(err) || errors.Is(err, whatsmeow.ErrIQTimedOut) || errors.Is(err, whatsmeow.ErrMessageTimedOut) || errors.Is(err, whatsmeow.ErrNotConnected),
		recipient: errors.Is(err, whatsmeow.ErrNotInGroup) || errors.Is(err, whatsmeow.ErrGroupNotFound) ||
			errors.Is(err, whatsmeow.ErrNoPermissionInGroup) || errors.Is(err, whatsmeow.ErrIQNotFound) ||
			errors.Is(err, whatsmeow.ErrIQForbidden) || errors.Is(err, whatsmeow.ErrRecipientADJID),
		err: err,
	}
}

func parseJID(s string) (types.JID, error) {
	j, err := types.ParseJID(s)
	if err != nil {
		return types.JID{}, &sendError{recipient: true, err: fmt.Errorf("parse jid: %w", err)}
	}
	return j, nil
}

func (rc *realClient) SendDocument(ctx context.Context, jid string, data []byte, doc document) error {
	to, err := parseJID(jid)
	if err != nil {
		return err
	}
	cli := rc.client()
	up, err := cli.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return classify(true, fmt.Errorf("upload: %w", err))
	}
	m := &waE2E.DocumentMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		Mimetype:      proto.String(doc.mime),
		FileName:      proto.String(doc.fileName),
		Title:         proto.String(doc.fileName),
	}
	if doc.caption != "" {
		m.Caption = proto.String(doc.caption)
	}
	if _, err := cli.SendMessage(ctx, to, &waE2E.Message{DocumentMessage: m}); err != nil {
		return classify(false, fmt.Errorf("send document: %w", err))
	}
	return nil
}

func (rc *realClient) SendText(ctx context.Context, jid, text string) error {
	to, err := parseJID(jid)
	if err != nil {
		return err
	}
	if _, err := rc.client().SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(text)}); err != nil {
		return classify(false, fmt.Errorf("send text: %w", err))
	}
	return nil
}

func (rc *realClient) Close() error {
	cli := rc.client()
	cli.Disconnect()
	cli.RemoveEventHandlers()
	if err := rc.container.Close(); err != nil {
		return fmt.Errorf("close session store: %w", err)
	}
	return nil
}

var _ waClient = (*realClient)(nil)
