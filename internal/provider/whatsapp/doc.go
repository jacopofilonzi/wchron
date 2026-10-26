// Package whatsapp is the WhatsApp provider: a linked device through
// whatsmeow (go.mau.fi/whatsmeow, pinned to a commit in go.mod; its API changes
// often, so read its source at that commit before changing this package).
// It is the only package that imports whatsmeow.
//
// Session: channels/<id>/session.db, a whatsmeow sqlstore container on
// modernc.org/sqlite (driver "sqlite", no CGO), opened with the
// foreign_keys pragma whatsmeow requires. A session.db that can't be opened
// is renamed to session.db.corrupt-<YYYYMMDD-HHMMSS> (with its -wal/-shm/
// -journal files), never deleted, and the channel starts unpaired with
// wa.session_corrupt. The session is never logged; phone numbers in the log
// are masked (see log.go, which adapts whatsmeow's logger to slog and drops
// its raw protocol dumps).
//
// Pairing: QR codes only, no WhatsApp Web view. StartPairing streams the QR
// contents (the GUI draws them, event channel:qr); the device shows on the
// phone as "wchron" (store.DeviceProps). The codes expire after about 2
// minutes in total (wa.pair_timeout); a refused or broken pairing is
// wa.pair_failed. Logout removes the linked device from the account and
// deletes the local session; the channel stays configured, unpaired.
//
// Connection: whatsmeow's own reconnect is turned off (EnableAutoReconnect
// false) so the provider.Manager's backoff (5 s doubling to 5 min) is the
// only one. Events map to the channel status:
//
//	Connected        → connected
//	Disconnected     → disconnected, retried (channel.disconnected)
//	ConnectFailure   → error, retried
//	keep-alive failing for 3 min → disconnect, retried
//	LoggedOut        → logged_out (wa.logged_out), session deleted by whatsmeow
//	ClientOutdated   → outdated (wa.client_outdated): the update check
//	StreamReplaced   → disconnected, not retried (wa.stream_replaced): another
//	                   process uses the same session; retrying would fight it
//	TemporaryBan     → error, not retried (wa.temporary_ban)
//
// History sync downloads are off (ManualHistorySyncDownload): wchron never
// reads messages.
//
// Targets: {"kind":"number","number":"+<E.164>"} (typed with the
// international prefix; spaces, dashes, dots and brackets are dropped;
// wa.number_invalid otherwise; checked with IsOnWhatsApp → wa.not_on_whatsapp)
// or {"kind":"group","jid":"<id>@g.us"} (picked from the account's groups;
// the JID is stored, never the name, so renames don't break anything;
// wa.group_not_found when the account is no longer in it). Both checks need
// the connection: offline, ValidateTarget returns the normalized target with
// recipient.check_unavailable and the Manager stores it unverified.
//
// Send: every attachment as a document with its file name and MIME type
// (from the extension), the text as the caption of the first one; a text
// alone goes as a plain message. Files above MaxFileSize (100 MB, a
// conservative app limit: WhatsApp allows more, but the whole file is held in
// memory to encrypt and upload it) are send.too_large. A network error gets
// one short retry; then send.upload_failed or send.failed. A target the
// service rejects (not in the group, no account) is send.recipient_invalid.
//
// Testability: the channel logic (status, targets, send) works on the small
// waClient interface (client.go); real.go adapts *whatsmeow.Client to it, so
// the tests drive a fake client and never touch the network.
package whatsapp
