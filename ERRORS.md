# Error codes

Every error, warning and skip reason the user can see. The code is stable (never renamed once released);
the text the user reads is `error.<code>` in `locales/it.json` / `locales/en.json`, filled with the params.
Logs always carry the code plus an English message.

Rules:

- Codes are dotted lowercase, `area.reason`. The registry is `internal/apperr/codes.go`; a test fails if
  the registry, both catalogs and this file disagree.
- **None of these is ever sent to a schedule's recipient.** Where a row says "Err", the text goes only to
  the error recipients (`settings.error_recipients`, on any channel), if set.
- Where: `GUI` inline in the window (form or dialog), `Dialog` native error box at startup (the app then exits
  without touching the data; title `dialog.fatal.title`), `Toast` Windows desktop notification, `Err` message
  to the error recipients, `Status` status page and log viewer, `Tray` tray icon/status line.
- Generic codes (`channel.*`, `recipient.*`, `send.*`) hold for every provider; a provider's own codes use
  its prefix (`wa.*` for WhatsApp; later `telegram.*`, `ntfy.*`). `channel`/`recipient` params are the
  user labels (channel name, recipient display name).
- Level: `error` (the action or file failed), `warning` (sent, but something may be off), `skip` (a slot
  or file was intentionally not sent).

## Storage and startup

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `storage.unavailable` | error | Dialog | `path` | The data dir (or one of its folders: `channels`, `logs`, `backups`, the temp folder) can't be created or written. The app doesn't start. |
| `storage.backup_failed` | error | Dialog | `path` | The backup before a migration/upgrade failed (`path`: the backup folder). The app doesn't start, to protect the data, and leaves the data files as they are. Free disk space or check permissions. |
| `config.read_failed` | error | Dialog | `path` | `config.json` exists but can't be read or parsed. The app doesn't start and leaves the file as is; restore it from `backups/`. |
| `config.invalid` | error | Dialog | `path`, `field` | `config.json` parses but breaks the structure (`field`: JSON path, e.g. `schedules[2].id`: empty or duplicate id, unknown theme, language, cadence mode, day or match mode), e.g. after a hand edit. The app doesn't start and leaves the file as is. |
| `config.write_failed` | error | GUI | `path` | Saving the configuration failed; the change is not applied (the previous file is intact). At startup (saving a migrated file) the app doesn't start (Dialog). |
| `config.newer_version` | error | Dialog | `found`, `supported` | `config.json` or `state.json` was written by a newer wchron. The app doesn't start and doesn't touch it: install the newer version. |
| `config.migration_failed` | error | Dialog | `from`, `to` | Upgrading the format of `config.json` or `state.json` failed; the app doesn't start and the original files are untouched. |
| `state.read_failed` | warning | Status | `path` | `state.json` unreadable: last-run info is reset; the bad file is kept as `state.json.corrupt-<timestamp>` next to it. Schedules keep working. |
| `state.write_failed` | warning | Status | `path` | Last-run info (or the routine pause) could not be saved; it is kept in memory until the next start. |
| `devenv.invalid` | error | Dialog | `name`, `value` | Development only: an env override (`WCHRON_LOG_LEVEL`, `WCHRON_UPDATE_DISABLED`…) has an invalid value. The app doesn't start. |
| `instance.already_running` | — | — | — | Not shown: a second launch just focuses the first window. Listed for the log. |

## Schedule editor (validation)

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `schedule.name_required` | error | GUI | — | Name is empty. |
| `schedule.no_files` | error | GUI | — | At least one file is needed. |
| `schedule.no_days` | error | GUI | — | No active day selected. |
| `schedule.not_found` | error | GUI, Status | `id` | The schedule was deleted meanwhile (e.g. from another view, or between its slot and its run: the run is skipped). |
| `schedule.busy` | error | GUI | `name` | "Send now" while a run of the same schedule is in progress. |
| `cadence.interval_invalid` | error | GUI | — | Interval is 0, longer than 24h, negative, or has more than 59 minutes. |
| `cadence.anchor_invalid` | error | GUI | `value` | Start time is not a valid `HH:MM`. |
| `cadence.window_invalid` | error | GUI | — | Window start and end are missing, not valid `HH:MM`, or equal. A start after the end is valid: the window crosses midnight (e.g. 22:00–02:00). |
| `cadence.window_empty` | error | GUI | — | No slot of the interval falls inside the window. |
| `cadence.no_times` | error | GUI | — | Fixed-times mode with no time. |
| `cadence.time_invalid` | error | GUI | `value` | A fixed time is not a valid `HH:MM`. |
| `cadence.time_duplicate` | error | GUI | `value` | The same fixed time appears twice. |
| `recipient.required` | error | GUI | — | No recipient chosen: a schedule needs at least one. |
| `recipient.duplicate` | error | GUI | `recipient` | The same recipient on the same channel appears twice. |
| `recipient.channel_missing` | error | GUI, Toast, Err, Status | `schedule`, `recipient` | The recipient's channel no longer exists (e.g. `config.json` edited by hand). At run time that recipient is skipped; for an error recipient the report to it is `notify.failed`. |
| `recipient.check_unavailable` | warning | GUI | `channel` | The recipient can't be checked now (its channel is not connected). It is saved anyway, as not verified (`verified: "unknown"`), and checked automatically when the channel connects. |
| `recipient.check_failed` | warning | Status | `recipient`, `channel` | An unverified recipient was checked when its channel connected and the service rejected it (e.g. the number is not on WhatsApp, the group was left). It is stored as `verified: "no"` and never sent to; fix or remove it in the schedule. |
| `recipient.target_invalid` | error | GUI | `channel`, `field` | A field of the recipient is missing or malformed for its provider. |
| `filespec.path_required` | error | GUI | — | A file has no path. |
| `filespec.path_not_absolute` | error | GUI | `value` | Paths must be absolute (`C:\…` or `\\server\…`). |
| `filespec.glob_in_dir` | error | GUI | `value` | Wildcards (`*`, `?`) are allowed only in the file name, not in folders. |
| `processor.unknown` | error | GUI | `type` | Unknown processor type (config edited by hand or from a newer version). |
| `processor.options_invalid` | error | GUI | `field` | A processor option is missing or wrong. |
| `filespec.send_name_invalid` | error | GUI, Toast, Err, Status | `value` | The name to send (`value`: as rendered, with the extension added) is not a valid Windows file name: it has one of `< > : " /  | ? *` or a control character, ends with a dot or a space, is a reserved name (`CON`, `PRN`, `AUX`, `NUL`, `COM1`–`9`, `LPT1`–`9`, also with an extension) or is longer than 255 characters. At run time (a date token that renders a `/`, a hand edit) that file is skipped. |
| `xlsx.not_xlsx` | error | GUI, Toast, Err, Status | `value` | "Values only" handles only `.xlsx`, `.xlsm`, `.xltx`, `.xltm` workbooks (`value`: the file name); old `.xls` and binary `.xlsb` files are not supported. At run time that file is skipped. Choose "Original" to send it as it is. |

## Tokens (paths, captions, sheet names)

Shown inline under the field, with the live preview (syntax in README.md "Files and dates in names").
`pos` is the 1-based character position in the template (characters, not bytes: `è` counts as one);
only the first error is reported.

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `token.unclosed` | error | GUI | `pos` | The `{` at `pos` has no matching `}` (the template ends, or another `{` comes first). Use `{{` for a literal brace. |
| `token.unexpected_close` | error | GUI | `pos` | A `}` outside a token. Use `}}` for a literal brace. |
| `token.unknown` | error | GUI | `pos`, `name` | Unknown token name (`pos`: its first character). Today the only one is `date` (lowercase). |
| `token.format_invalid` | error | GUI | `pos`, `format` | The format after `:` has a letter that is not a format code, or is empty (`pos`: the first bad letter; `format`: the whole format). |
| `token.offset_invalid` | error | GUI | `pos`, `offset` | A modifier is not understood (`offset`: the modifier as written, `pos`: its `+`, `-` or `@`). Offsets are `+N`/`-N` with 1–4 digits and a unit `d` `w` `m` `y`; anchors are `@month-start`, `@month-end`, `@week-start`, `@week-end`, `@year-start`, `@year-end`. |
| `token.empty` | error | GUI | `pos` | `{}` (or only spaces) with nothing inside. |

## Run time (scheduled runs and "send now")

Collected per run: one Toast (and one message to each error recipient, if any) lists all the problems of
that run, per file and recipient. The provider fills the params it knows (`file`, `size`, `max`); the
pipeline adds `schedule`, `channel` and `recipient`. The same problem set (same codes for the same files,
recipients and channels) is reported again at most every 6 hours; when the schedule works again a
"works again" notice follows (`notify.*` texts). A "send now" reports with a Toast only, every time. At run
time the `filespec.*`, `processor.*` and `token.*` codes of a hand-edited schedule (a path or caption that
doesn't render) also show up here, as skips of the files concerned.

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `file.not_found` | skip | Toast, Err, Status | `schedule`, `path` | No file matches the pattern now. Tried again at the next slot. |
| `file.multiple_matches` | skip | Toast, Err, Status | `schedule`, `path`, `count` | The glob matched several files and the spec is in "error" mode. Choose "newest" or "all", or make the pattern stricter. |
| `file.locked` | skip | Toast, Err, Status | `schedule`, `file` | The file is locked by another program (e.g. being saved). |
| `file.unreadable` | skip | Toast, Err, Status | `schedule`, `file` | Access denied or read error (also a folder of the pattern that can't be listed). |
| `xlsx.open_failed` | skip | Toast, Err, Status, GUI | `schedule`, `file` | Not a valid workbook, corrupted, or protected by a password (encrypted workbooks are not supported). Also from "read the sheets from the file" in the editor. |
| `xlsx.sheet_missing` | warning | Toast, Err, Status | `schedule`, `file`, `sheet` | A listed sheet (`sheet`: its name as rendered at the slot) is not in the file. The file is still sent with the sheets that were found; one warning per missing sheet. |
| `xlsx.no_sheet_found` | skip | Toast, Err, Status | `schedule`, `file`, `sheets` | None of the listed sheets (`sheets`: comma-separated, as rendered) is in the file: it is not sent. |
| `xlsx.no_cached_values` | warning | Toast, Err, Status | `schedule`, `file`, `sheet`, `count` | Sent, but `count` formula cells of `sheet` had no saved value (file generated by a script, not saved by Excel): they are blank. wchron never recalculates formulas. One warning per sheet. |
| `xlsx.process_failed` | skip | Toast, Err, Status | `schedule`, `file` | Making the values-only copy failed while editing or saving it (details in the log). |
| `temp.unavailable` | skip | Toast, Err, Status | `schedule` | The temp folder (`%TEMP%\wchron\<run>`) can't be written: nothing (or that file) is sent. |
| `send.not_connected` | skip | Toast, Err, Tray, Status | `schedule`, `channel`, `recipient` | The recipient's channel is not connected at slot time. Err only reaches error recipients on other, connected channels. |
| `send.too_large` | skip | Toast, Err, Status | `schedule`, `file`, `channel`, `size`, `max` | The file is above the channel's size limit; skipped for that recipient only. |
| `send.caption_too_long` | skip | Toast, Err, Status | `schedule`, `file`, `channel`, `recipient`, `max` | The caption is longer than the channel allows (`max` characters); that file is skipped for that recipient only. |
| `send.files_unsupported` | skip | Toast, Err, Status | `schedule`, `channel`, `recipient` | The channel can't send files (text-only provider): nothing is sent to that recipient. |
| `send.upload_failed` | skip | Toast, Err, Status | `schedule`, `file`, `recipient` | Upload failed after one short retry. |
| `send.failed` | skip | Toast, Err, Status | `schedule`, `file`, `recipient` | The service refused or didn't acknowledge the message after one short retry. |
| `send.recipient_invalid` | skip | Toast, Err, Status | `schedule`, `recipient`, `channel` | The service rejected the recipient (number deactivated, removed from the group, chat deleted…), at the send or at the check right before it; or the recipient is stored as not verified ("no", it failed an earlier check) and is never sent to. Fix or remove it in the schedule. |

## Skip reasons (status and log)

Not errors: shown in the schedule's last result and in the log, never as a Toast. `slot`, `from` and `to`
are local wall-clock times `YYYY-MM-DD HH:MM`. Missed and paused slots are aggregated: one entry per
schedule for a whole stretch, not one per slot.

| Code | Level | Where | Params | Meaning |
| --- | --- | --- | --- | --- |
| `slot.missed` | skip | Status | `schedule`, `count`, `from`, `to` | `count` slots (the first at `from`, the last at `to`) passed more than 5 minutes ago when the scheduler looked (PC asleep or off, clock moved forward); never recovered. Slots that passed while the app was closed are not reported. |
| `slot.late` | skip | Status | `schedule`, `slot` | The run would start more than 5 minutes after the slot; skipped. |
| `slot.dst_gap` | skip | Status | `schedule`, `slot` | The slot falls in the hour skipped by the spring DST change. |
| `slot.dst_repeat` | skip | Status | `schedule`, `slot` | Second occurrence of a slot in the hour repeated by the autumn DST change (it ran, or was due, at the first). |
| `slot.paused` | skip | Status | `schedule`, `count`, `from`, `to` | `count` slots came while the routine was paused from the tray (one entry per paused stretch). |
| `slot.overlap` | skip | Status | `schedule`, `slot` | The previous run of the same schedule was still in progress at the slot. Different schedules run in parallel; a schedule never overlaps itself. |

## Channels (any provider)

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `channel.name_required` | error | GUI | — | The channel needs a name. |
| `channel.provider_unknown` | error | GUI | `provider` | Unknown provider (config edited by hand or from a newer version). |
| `channel.settings_invalid` | error | GUI | `channel`, `field` | A setting of the channel is missing or wrong. |
| `channel.not_found` | error | GUI | `id` | The channel was deleted meanwhile. |
| `channel.not_connected` | error | GUI | `channel` | An action that needs the connection (target list, recipient check, send now) while disconnected. |
| `channel.disconnected` | error | Toast, Tray, Status | `channel` | Connection lost; the app reconnects by itself with backoff. |
| `channel.auth_failed` | error | Toast, Tray, GUI | `channel` | The service refused the credentials or the session (logged out, token revoked). Set the channel up again. |
| `channel.outdated` | error | Toast, Tray, GUI | `channel` | The service rejected this version of the app. An update check starts; install the update when offered. |
| `channel.logout_failed` | error | GUI | `channel` | Logging out (removing the linked device from the service) failed. Check the connection and try again, or remove the device from the phone. |
| `channel.targets_failed` | error | GUI | `channel` | The list of groups/chats could not be loaded. |
| `channel.secret_unavailable` | error | GUI, Toast | `channel` | The channel's stored secret can't be decrypted (e.g. Windows user profile changed). Enter it again. |

## WhatsApp provider

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `wa.number_invalid` | error | GUI | `value` | Not a valid international number (needs the country prefix). |
| `wa.not_on_whatsapp` | error | GUI | `value` | The number has no WhatsApp account. |
| `wa.group_not_found` | error | GUI, Toast, Err | `channel`, `name` | The group is no longer among the account's groups (left or removed). At run time that recipient is skipped. `name` is the group JID when its name is not known. |
| `wa.client_outdated` | error | Toast, Tray, GUI | `channel` | WhatsApp rejected this version of whatsmeow. An update check starts; install the update when offered. |
| `wa.logged_out` | error | Toast, Tray, GUI | `channel` | The linked device was removed from the phone or expired. Pair again from the Channels page. |
| `wa.pair_timeout` | error | GUI | `channel` | QR codes expired without a scan. Start again. |
| `wa.pair_failed` | error | GUI | `channel` | Pairing refused or interrupted. |
| `wa.stream_replaced` | error | Toast, Tray, GUI | `channel` | The same WhatsApp session was opened elsewhere (e.g. a copy of wchron using the same data folder), so this one was disconnected. wchron doesn't reconnect by itself, to avoid a tug of war: close the other one, then restart wchron. |
| `wa.temporary_ban` | error | Toast, Tray, GUI | `channel` | WhatsApp temporarily blocked the account. wchron doesn't reconnect by itself; wait, then restart wchron. |
| `wa.session_corrupt` | error | GUI, Toast | `channel` | The channel's `session.db` can't be opened. It is renamed to `session.db.corrupt-<timestamp>` in the channel dir and a new pairing is needed. |

## Error recipients

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `notify.failed` | warning | Toast, Status | `recipient` | Sending the error report (or the "works again" notice) to an error recipient failed: its channel is missing or not connected, or the send failed. The run's toast is still shown, and a second toast lists the undelivered reports. |

## Platform

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `autostart.failed` | error | GUI | — | Couldn't enable/disable start at login (registry write refused). |
| `keepawake.failed` | error | Tray, Toast | — | Windows refused to keep the PC awake. |
| `logs.read_failed` | error | GUI | — | The log files can't be read. |

## Updates

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `update.check_failed` | error | GUI | — | Only on a manual check; the automatic one fails silently (offline, GitHub down). |
| `update.download_failed` | error | GUI, Toast | `version` | The installer could not be downloaded. |
| `update.verify_failed` | error | GUI, Toast | `version` | The downloaded installer doesn't match its checksum (corrupted download); it is deleted and not run. Try again. |
| `update.install_failed` | error | GUI, Toast | `version` | The installer could not be started. |

## Generic

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `internal` | error | GUI, Toast | — | Unexpected error; details are in the log. Always a bug to fix with a specific code. |
