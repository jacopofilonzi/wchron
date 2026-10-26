package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/provider"
)

// fakeClient is a scripted waClient. Numbers and JIDs are synthetic.
type fakeClient struct {
	mu        sync.Mutex
	handler   func(waEvent)
	paired    bool
	number    string
	connects  int
	disc      int
	connErr   error
	onWA      map[string]bool
	waErr     error
	groups    []group
	groupsErr error
	qr        chan qrItem
	sendErrs  []error // returned by the next sends, in order
	sends     []sentDoc
	texts     []string
	logoutErr error
	closed    bool
}

type sentDoc struct {
	jid  string
	size int
	doc  document
}

func (f *fakeClient) SetHandler(h func(waEvent)) { f.mu.Lock(); f.handler = h; f.mu.Unlock() }
func (f *fakeClient) emit(ev waEvent) {
	f.mu.Lock()
	h := f.handler
	f.mu.Unlock()
	h(ev)
}
func (f *fakeClient) IsPaired() bool    { f.mu.Lock(); defer f.mu.Unlock(); return f.paired }
func (f *fakeClient) OwnNumber() string { f.mu.Lock(); defer f.mu.Unlock(); return f.number }
func (f *fakeClient) Connect() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connects++
	return f.connErr
}
func (f *fakeClient) Disconnect() { f.mu.Lock(); f.disc++; f.mu.Unlock() }
func (f *fakeClient) Logout(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logoutErr != nil {
		return f.logoutErr
	}
	f.paired, f.number = false, ""
	return nil
}
func (f *fakeClient) QRChannel(context.Context) (<-chan qrItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.qr = make(chan qrItem, 8)
	return f.qr, nil
}
func (f *fakeClient) IsOnWhatsApp(_ context.Context, n string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.onWA[n], f.waErr
}
func (f *fakeClient) JoinedGroups(context.Context) ([]group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]group(nil), f.groups...), f.groupsErr
}
func (f *fakeClient) next() error {
	if len(f.sendErrs) == 0 {
		return nil
	}
	err := f.sendErrs[0]
	f.sendErrs = f.sendErrs[1:]
	return err
}
func (f *fakeClient) SendDocument(_ context.Context, jid string, data []byte, doc document) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, sentDoc{jid: jid, size: len(data), doc: doc})
	return f.next()
}
func (f *fakeClient) SendText(_ context.Context, jid, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texts = append(f.texts, jid+" "+text)
	return f.next()
}
func (f *fakeClient) Close() error { f.mu.Lock(); f.closed = true; f.mu.Unlock(); return nil }

type statuses struct {
	mu   sync.Mutex
	list []provider.Status
}

func (s *statuses) add(st provider.Status) { s.mu.Lock(); s.list = append(s.list, st); s.mu.Unlock() }
func (s *statuses) last() provider.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.list) == 0 {
		return provider.Status{}
	}
	return s.list[len(s.list)-1]
}

func newTestChannel(t *testing.T, f *fakeClient) (*channel, *statuses, *clock.Fake) {
	t.Helper()
	st := &statuses{}
	clk := clock.NewFake(time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC))
	c := newChannel(f, provider.Deps{Clock: clk, OnStatus: st.add})
	t.Cleanup(func() { _ = c.Close() })
	return c, st, clk
}

func code(err error) apperr.Code { return apperr.As(err).Code }

func TestNormalizeNumber(t *testing.T) {
	ok := map[string]string{
		"+39 333 123 4567":   "+393331234567",
		" +39-333-1234567 ":  "+393331234567",
		"0039 (333) 1234567": "+393331234567",
		"+1.555.000.0000":    "+15550000000",
	}
	for in, want := range ok {
		if got, err := normalizeNumber(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"333 1234567", "+0393331234567", "+39 333", "+39 333 12a4567", "+1234567890123456", ""} {
		_, err := normalizeNumber(in)
		if e := apperr.As(err); e.Code != apperr.WANumberInvalid || e.Params["value"] != strings.TrimSpace(in) {
			t.Errorf("%q: got %v", in, err)
		}
		if err != nil && strings.Contains(err.Error(), "1234567") {
			t.Errorf("%q: the log message must mask the number: %v", in, err)
		}
	}
}

func TestParseTarget(t *testing.T) {
	tg, err := parseTarget(json.RawMessage(`{"kind":"number","number":"+39 333 1234567"}`))
	if err != nil || string(tg.raw()) != `{"kind":"number","number":"+393331234567"}` || tg.jid() != "393331234567@s.whatsapp.net" {
		t.Fatalf("number: %+v %v", tg, err)
	}
	tg, err = parseTarget(json.RawMessage(`{"kind":"group","jid":" 120363000000000000@g.us "}`))
	if err != nil || string(tg.raw()) != `{"kind":"group","jid":"120363000000000000@g.us"}` {
		t.Fatalf("group: %+v %v", tg, err)
	}
	bad := map[string]string{
		`{"kind":"group","jid":"393331234567@s.whatsapp.net"}`: "jid",
		`{"kind":"group"}`: "jid",
		`{"kind":"email"}`: "kind",
		`[]`:               "kind",
	}
	for raw, field := range bad {
		_, err := parseTarget(json.RawMessage(raw))
		if e := apperr.As(err); e.Code != apperr.RecipientTargetInvalid || e.Params["field"] != field {
			t.Errorf("%s: got %v %v", raw, err, e.Params)
		}
	}
}

func TestStatusFromEvents(t *testing.T) {
	f := &fakeClient{paired: true, number: "393330000123"}
	c, st, clk := newTestChannel(t, f)
	if s := c.Status(); s.State != provider.StateDisconnected || s.Account != "***123" {
		t.Fatalf("initial: %+v", s)
	}
	if err := c.Connect(t.Context()); err != nil || c.Status().State != provider.StateConnecting {
		t.Fatalf("connect: %v %+v", err, c.Status())
	}
	f.emit(waEvent{kind: evConnected})
	if s := st.last(); s.State != provider.StateConnected || s.Error != nil {
		t.Fatalf("connected: %+v", s)
	}
	f.emit(waEvent{kind: evDisconnected})
	if s := st.last(); s.State != provider.StateDisconnected || !s.Retry || s.Error.Code != apperr.ChannelDisconnected {
		t.Fatalf("disconnected: %+v", s)
	}
	f.emit(waEvent{kind: evConnected})
	// Keep-alive failing for a short while: nothing; for more than 3 minutes: reconnect.
	f.emit(waEvent{kind: evKeepAliveTimeout, lastSuccess: clk.Now().Add(-time.Minute)})
	if st.last().State != provider.StateConnected {
		t.Fatal("a short keep-alive failure must not disconnect")
	}
	f.emit(waEvent{kind: evKeepAliveTimeout, lastSuccess: clk.Now().Add(-4 * time.Minute)})
	deadline := time.Now().Add(2 * time.Second)
	for st.last().State != provider.StateDisconnected && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s := st.last(); s.State != provider.StateDisconnected || !s.Retry {
		t.Fatalf("keep-alive: %+v", s)
	}

	f.emit(waEvent{kind: evStreamReplaced})
	if s := st.last(); s.State != provider.StateDisconnected || s.Retry || s.Error.Code != apperr.WAStreamReplaced {
		t.Fatalf("replaced: %+v", s)
	}
	f.emit(waEvent{kind: evDisconnected})
	if s := st.last(); s.Retry || s.Error.Code != apperr.WAStreamReplaced {
		t.Fatalf("a replaced stream must stay final: %+v", s)
	}

	f.emit(waEvent{kind: evOutdated})
	if s := st.last(); s.State != provider.StateOutdated || s.Retry || s.Error.Code != apperr.WAClientOutdated {
		t.Fatalf("outdated: %+v", s)
	}
	f.paired = false
	f.emit(waEvent{kind: evLoggedOut})
	if s := st.last(); s.State != provider.StateLoggedOut || s.Account != "" || s.Error.Code != apperr.WALoggedOut {
		t.Fatalf("logged out: %+v", s)
	}
	if err := c.Connect(t.Context()); err != nil || c.Status().State != provider.StateLoggedOut {
		t.Fatalf("connect after logout keeps logged_out: %v %+v", err, c.Status())
	}
}

func TestConnectFailureAndUnpaired(t *testing.T) {
	f := &fakeClient{paired: true, number: "393330000123", connErr: &net.OpError{Op: "dial", Err: errors.New("offline")}}
	c, _, _ := newTestChannel(t, f)
	if err := c.Connect(t.Context()); err == nil {
		t.Fatal("want an error")
	}
	if s := c.Status(); s.State != provider.StateDisconnected || !s.Retry || s.Error.Code != apperr.ChannelDisconnected {
		t.Fatalf("status: %+v", s)
	}
	u, _, _ := newTestChannel(t, &fakeClient{})
	if err := u.Connect(t.Context()); err != nil || u.Status().State != provider.StateNotPaired {
		t.Fatalf("unpaired: %v %+v", err, u.Status())
	}
}

func collect(t *testing.T, ch <-chan provider.PairEvent) []provider.PairEvent {
	t.Helper()
	var out []provider.PairEvent
	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			t.Fatal("pairing did not end")
		}
	}
}

func TestPairing(t *testing.T) {
	f := &fakeClient{}
	c, st, _ := newTestChannel(t, f)
	events, err := c.StartPairing(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	f.qr <- qrItem{event: "code", code: "2@abc"}
	f.qr <- qrItem{event: "code", code: "2@def"}
	f.emit(waEvent{kind: evPairSuccess, account: "393330000456"})
	f.qr <- qrItem{event: "success"}
	got := collect(t, events)
	if len(got) != 3 || got[0].Code != "2@abc" || got[1].Code != "2@def" || got[2].Kind != provider.PairSuccess {
		t.Fatalf("events: %+v", got)
	}
	if s := st.last(); s.State != provider.StateConnecting || s.Account != "***456" {
		t.Fatalf("after pair success: %+v", s)
	}
	f.paired = true
	f.emit(waEvent{kind: evConnected})
	if c.Status().State != provider.StateConnected {
		t.Fatal("connected after pairing")
	}
	if _, err := c.StartPairing(t.Context()); err == nil {
		t.Fatal("pairing a paired channel must fail")
	}
}

func TestPairingTimeoutFailureCancel(t *testing.T) {
	f := &fakeClient{}
	c, _, _ := newTestChannel(t, f)
	events, _ := c.StartPairing(t.Context())
	f.qr <- qrItem{event: "timeout"}
	got := collect(t, events)
	if len(got) != 1 || got[0].Kind != provider.PairTimeout || code(got[0].Err) != apperr.WAPairTimeout {
		t.Fatalf("timeout: %+v", got)
	}
	events, _ = c.StartPairing(t.Context())
	f.qr <- qrItem{event: "error", err: errors.New("refused")}
	got = collect(t, events)
	if len(got) != 1 || got[0].Kind != provider.PairFailed || code(got[0].Err) != apperr.WAPairFailed {
		t.Fatalf("failure: %+v", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	events, _ = c.StartPairing(ctx)
	f.qr <- qrItem{event: "code", code: "x"}
	<-events
	before := f.disc
	cancel()
	collect(t, events)
	if f.disc <= before {
		t.Fatal("cancel must close the pairing socket")
	}
}

func connectedChannel(t *testing.T, f *fakeClient) *channel {
	t.Helper()
	f.paired, f.number = true, "393330000123"
	c, _, _ := newTestChannel(t, f)
	_ = c.Connect(t.Context())
	f.emit(waEvent{kind: evConnected})
	return c
}

func TestValidateTarget(t *testing.T) {
	f := &fakeClient{onWA: map[string]bool{"+393331234567": true},
		groups: []group{{jid: "120363000000000001@g.us", name: "Team"}}}
	off, _, _ := newTestChannel(t, &fakeClient{paired: true})
	norm, _, err := off.ValidateTarget(t.Context(), json.RawMessage(`{"kind":"number","number":"+39 333 1234567"}`))
	if code(err) != apperr.RecipientCheckUnavailable || string(norm) != `{"kind":"number","number":"+393331234567"}` {
		t.Fatalf("offline: %s %v", norm, err)
	}
	if _, _, err := off.ValidateTarget(t.Context(), json.RawMessage(`{"kind":"number","number":"333"}`)); code(err) != apperr.WANumberInvalid {
		t.Fatalf("offline, bad format: %v", err)
	}

	c := connectedChannel(t, f)
	norm, name, err := c.ValidateTarget(t.Context(), json.RawMessage(`{"kind":"number","number":"+39 333 1234567"}`))
	if err != nil || name != "+393331234567" || string(norm) != `{"kind":"number","number":"+393331234567"}` {
		t.Fatalf("number: %s %q %v", norm, name, err)
	}
	_, _, err = c.ValidateTarget(t.Context(), json.RawMessage(`{"kind":"number","number":"+39 333 7654321"}`))
	if code(err) != apperr.WANotOnWhatsApp || !errors.Is(err, provider.ErrTargetInvalid) {
		t.Fatalf("not on whatsapp: %v", err)
	}
	if norm, name, err = c.ValidateTarget(t.Context(), json.RawMessage(`{"kind":"group","jid":"120363000000000001@g.us"}`)); err != nil || name != "Team" {
		t.Fatalf("group: %s %q %v", norm, name, err)
	}
	_, _, err = c.ValidateTarget(t.Context(), json.RawMessage(`{"kind":"group","jid":"120363000000000009@g.us"}`))
	if code(err) != apperr.WAGroupNotFound || !errors.Is(err, provider.ErrTargetInvalid) {
		t.Fatalf("group not found: %v", err)
	}
	f.waErr = errors.New("timeout")
	if _, _, err := c.ValidateTarget(t.Context(), json.RawMessage(`{"kind":"number","number":"+393331234567"}`)); code(err) != apperr.RecipientCheckUnavailable {
		t.Fatalf("temporary failure: %v", err)
	}
}

func TestListTargets(t *testing.T) {
	f := &fakeClient{groups: []group{{jid: "2@g.us", name: "beta"}, {jid: "1@g.us", name: "Alpha"}}}
	off, _, _ := newTestChannel(t, &fakeClient{})
	if _, err := off.ListTargets(t.Context()); code(err) != apperr.ChannelNotConnected {
		t.Fatalf("offline: %v", err)
	}
	c := connectedChannel(t, f)
	ts, err := c.ListTargets(t.Context())
	if err != nil || len(ts) != 2 || ts[0].DisplayName != "Alpha" || string(ts[0].Target) != `{"kind":"group","jid":"1@g.us"}` {
		t.Fatalf("targets: %+v %v", ts, err)
	}
	f.groupsErr = errors.New("iq failed")
	if _, err := c.ListTargets(t.Context()); code(err) != apperr.ChannelTargetsFailed {
		t.Fatalf("failure: %v", err)
	}
}

func writeFile(t *testing.T, name string, size int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSend(t *testing.T) {
	retryDelay = 0
	f := &fakeClient{}
	num := json.RawMessage(`{"kind":"number","number":"+393331234567"}`)
	off, _, _ := newTestChannel(t, &fakeClient{paired: true})
	if err := off.Send(t.Context(), num, provider.Message{Text: "x"}); code(err) != apperr.SendNotConnected {
		t.Fatalf("offline: %v", err)
	}
	c := connectedChannel(t, f)
	a := writeFile(t, "report.xlsx", 10)
	b := writeFile(t, "notes.bin", 20)
	err := c.Send(t.Context(), num, provider.Message{Text: "Report", Attachments: []provider.Attachment{
		{Path: a, Name: "Report 26-10.xlsx"}, {Path: b, Name: "notes.bin"},
	}})
	if err != nil || len(f.sends) != 2 {
		t.Fatalf("send: %v %+v", err, f.sends)
	}
	if s := f.sends[0]; s.jid != "393331234567@s.whatsapp.net" || s.size != 10 || s.doc.fileName != "Report 26-10.xlsx" ||
		s.doc.caption != "Report" || s.doc.mime != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("first: %+v", s)
	}
	if s := f.sends[1]; s.doc.caption != "" || s.doc.mime != "application/octet-stream" {
		t.Fatalf("second: %+v", s)
	}
	if err := c.Send(t.Context(), json.RawMessage(`{"kind":"group","jid":"1@g.us"}`), provider.Message{Text: "hi"}); err != nil || f.texts[0] != "1@g.us hi" {
		t.Fatalf("text: %v %v", err, f.texts)
	}

	// One retry on a network error, then the code of the step.
	f.sends = nil
	netErr := &net.OpError{Op: "write", Err: errors.New("reset")}
	f.sendErrs = []error{&sendError{upload: true, network: true, err: netErr}, nil}
	if err := c.Send(t.Context(), num, provider.Message{Attachments: []provider.Attachment{{Path: a, Name: "a.xlsx"}}}); err != nil || len(f.sends) != 2 {
		t.Fatalf("retried: %v %d", err, len(f.sends))
	}
	f.sends = nil
	f.sendErrs = []error{&sendError{upload: true, network: true, err: netErr}, &sendError{upload: true, network: true, err: netErr}}
	err = c.Send(t.Context(), num, provider.Message{Attachments: []provider.Attachment{{Path: a, Name: "a.xlsx"}}})
	if e := apperr.As(err); e.Code != apperr.SendUploadFailed || e.Params["file"] != "a.xlsx" || len(f.sends) != 2 {
		t.Fatalf("upload failed: %v %d", err, len(f.sends))
	}
	f.sends = nil
	f.sendErrs = []error{&sendError{err: errors.New("server error")}}
	if err := c.Send(t.Context(), num, provider.Message{Attachments: []provider.Attachment{{Path: a}}}); code(err) != apperr.SendFailed || len(f.sends) != 1 {
		t.Fatalf("no retry on a non-network failure: %v %d", err, len(f.sends))
	}
	f.sendErrs = []error{&sendError{recipient: true, err: errors.New("not in group")}}
	err = c.Send(t.Context(), num, provider.Message{Attachments: []provider.Attachment{{Path: a}}})
	if code(err) != apperr.SendRecipientInvalid || !errors.Is(err, provider.ErrTargetInvalid) {
		t.Fatalf("recipient invalid: %v", err)
	}
}

func TestSendTooLarge(t *testing.T) {
	f := &fakeClient{}
	c := connectedChannel(t, f)
	p := filepath.Join(t.TempDir(), "big.pdf")
	fh, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := fh.Truncate(MaxFileSize + 1); err != nil { // sparse: no 100 MB written
		t.Fatal(err)
	}
	_ = fh.Close()
	err = c.Send(t.Context(), json.RawMessage(`{"kind":"number","number":"+393331234567"}`), provider.Message{Attachments: []provider.Attachment{{Path: p, Name: "big.pdf"}}})
	if e := apperr.As(err); e.Code != apperr.SendTooLarge || e.Params["max"] != "100.0 MB" || e.Params["size"] != "100.0 MB" || e.Params["file"] != "big.pdf" || len(f.sends) != 0 {
		t.Fatalf("too large: %v %v", err, e.Params)
	}
}

func TestLogout(t *testing.T) {
	off, _, _ := newTestChannel(t, &fakeClient{paired: true})
	if err := off.Logout(t.Context()); code(err) != apperr.ChannelNotConnected {
		t.Fatalf("offline: %v", err)
	}
	f := &fakeClient{}
	c := connectedChannel(t, f)
	f.logoutErr = errors.New("iq timeout")
	if err := c.Logout(t.Context()); code(err) != apperr.ChannelLogoutFailed {
		t.Fatalf("failure: %v", err)
	}
	f.logoutErr = nil
	if err := c.Logout(t.Context()); err != nil || c.Status().State != provider.StateNotPaired || c.Status().Account != "" {
		t.Fatalf("logout: %v %+v", err, c.Status())
	}
}

func TestMaskNumbers(t *testing.T) {
	got := maskNumbers("sending to 393331234567@s.whatsapp.net and +393337654321, id 3EB0A1, port 443")
	if strings.Contains(got, "1234567") || strings.Contains(got, "7654321") ||
		got != "sending to ***567@s.whatsapp.net and ***321, id 3EB0A1, port 443" {
		t.Fatalf("got %q", got)
	}
	if logging.MaskPhone("+393331234567") != "***567" {
		t.Fatal("same masking as logging.MaskPhone")
	}
}

func TestSessionOpenAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	log := newLogger(logging.New(os.Stderr, 100)) // effectively silent
	rc, corrupt, err := openSession(t.Context(), dir, nil, log)
	if err != nil || corrupt || rc.IsPaired() {
		t.Fatalf("fresh: %v corrupt=%v", err, corrupt)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	// Opening again works (the schema is in place).
	rc, corrupt, err = openSession(t.Context(), dir, nil, log)
	if err != nil || corrupt {
		t.Fatalf("reopen: %v %v", err, corrupt)
	}
	_ = rc.Close()

	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, SessionFile), []byte("this is not a database, just junk bytes ..............."), 0o600); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFake(time.Date(2026, 10, 26, 9, 30, 0, 0, time.Local))
	rc, corrupt, err = openSession(t.Context(), bad, clk, log)
	if err != nil || !corrupt || rc.IsPaired() {
		t.Fatalf("corrupt: %v %v", err, corrupt)
	}
	_ = rc.Close()
	kept, err := os.ReadFile(filepath.Join(bad, SessionFile+".corrupt-20261026-093000"))
	if err != nil || !strings.HasPrefix(string(kept), "this is not") {
		t.Fatalf("the corrupt file must be kept aside: %v", err)
	}
}

func TestProviderOpenCorruptSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, SessionFile), []byte("junk junk junk junk junk junk junk junk junk junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	ch, err := New().Open(testChannelConfig(), dir, provider.Deps{AppVersion: "0.5.0"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ch.Close() }()
	if s := ch.Status(); s.State != provider.StateNotPaired || s.Error == nil || s.Error.Code != apperr.WASessionCorrupt {
		t.Fatalf("status: %+v", s)
	}
}

func testChannelConfig() config.Channel {
	return config.Channel{ID: "c1", Provider: Type, Name: "Test", Enabled: true, Settings: json.RawMessage(`{}`)}
}
