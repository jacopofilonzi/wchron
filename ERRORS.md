# Error codes

Every error, warning and skip reason the user can see. The code is stable (never renamed once released);
the text the user reads is `error.<code>` in `locales/it.json` / `locales/en.json`, filled with the params.
Logs always carry the code plus an English message.

Rules:

- Codes are dotted lowercase, `area.reason`. The registry is `internal/apperr/codes.go`; a test fails if
  the registry, both catalogs and this file disagree.
- **None of these is ever sent to a schedule's recipient.** Where a row says "Err", the text goes only to
  the error recipients (`settings.error_recipients`, on any channel), if set.
- Where: `GUI` inline in the window (form or dialog), `Toast` Windows desktop notification, `Err` message
  to the error recipients, `Status` status page and log viewer, `Tray` tray icon/status line.
- Generic codes (`channel.*`, `recipient.*`, `send.*`) hold for every provider; a provider's own codes use
  its prefix (`wa.*` for WhatsApp; later `telegram.*`, `ntfy.*`). `channel`/`recipient` params are the
  user labels (channel name, recipient display name).
- Level: `error` (the action or file failed), `warning` (sent, but something may be off), `skip` (a slot
  or file was intentionally not sent).

## Storage and startup

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `storage.unavailable` | error | Toast | `path` | The data dir can't be created or written. The app doesn't start. |
| `storage.backup_failed` | error | Toast | `path` | Backup before a migration/upgrade failed. The app doesn't start, to protect the data. Free disk space or check permissions. |
| `config.read_failed` | error | Toast | `path` | `config.json` exists but can't be read or parsed. The app doesn't start and leaves the file as is; restore it from `backups/`. |
| `config.write_failed` | error | GUI, Toast | `path` | Saving the configuration failed; the change is not applied. |
| `config.newer_version` | error | Toast | `found`, `supported` | The file was written by a newer wchron. The app doesn't start and doesn't touch it: install the newer version. |
| `config.migration_failed` | error | Toast | `from`, `to` | Upgrading the data format failed; the original files are untouched and backed up. |
| `state.read_failed` | warning | Status | `path` | `state.json` unreadable: last-run info is reset (backup kept). Schedules keep working. |
| `state.write_failed` | warning | Status | `path` | Last-run info could not be saved. |
| `instance.already_running` | — | — | — | Not shown: a second launch just focuses the first window. Listed for the log. |

## Schedule editor (validation)

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `schedule.name_required` | error | GUI | — | Name is empty. |
| `schedule.no_files` | error | GUI | — | At least one file is needed. |
| `schedule.no_days` | error | GUI | — | No active day selected. |
| `schedule.not_found` | error | GUI | `id` | The schedule was deleted meanwhile (e.g. from another view). |
| `schedule.busy` | error | GUI | `name` | "Send now" while a run of the same schedule is in progress. |
| `cadence.interval_invalid` | error | GUI | — | Interval is 0 or longer than 24h. |
| `cadence.anchor_invalid` | error | GUI | `value` | Start time is not a valid `HH:MM`. |
| `cadence.window_invalid` | error | GUI | — | Window start is not before window end. |
| `cadence.window_empty` | error | GUI | — | No slot of the interval falls inside the window. |
| `cadence.no_times` | error | GUI | — | Fixed-times mode with no time. |
| `cadence.time_invalid` | error | GUI | `value` | A fixed time is not a valid `HH:MM`. |
| `cadence.time_duplicate` | error | GUI | `value` | The same fixed time appears twice. |
| `recipient.required` | error | GUI | — | No recipient chosen: a schedule needs at least one. |
| `recipient.duplicate` | error | GUI | `recipient` | The same recipient on the same channel appears twice. |
| `recipient.channel_missing` | error | GUI, Toast, Err, Status | `recipient` | The recipient's channel no longer exists (e.g. `config.json` edited by hand). At run time that recipient is skipped. |
| `recipient.check_unavailable` | error | GUI | `channel` | The recipient can't be checked now (its channel is not connected). Connect and save again. |
| `recipient.target_invalid` | error | GUI | `channel`, `field` | A field of the recipient is missing or malformed for its provider. |
| `filespec.path_required` | error | GUI | — | A file has no path. |
| `filespec.path_not_absolute` | error | GUI | `value` | Paths must be absolute (`C:\…` or `\\server\…`). |
| `filespec.glob_in_dir` | error | GUI | `value` | Wildcards (`*`, `?`) are allowed only in the file name, not in folders. |
| `processor.unknown` | error | GUI | `type` | Unknown processor type (config edited by hand or from a newer version). |
| `processor.options_invalid` | error | GUI | `field` | A processor option is missing or wrong. |
| `xlsx.no_sheets` | error | GUI | — | The Excel processor needs at least one sheet name. |
| `xlsx.not_xlsx` | error | GUI | `value` | The Excel processor only handles `.xlsx`/`.xlsm` files. |

## Tokens (paths, captions, sheet names)

Shown inline under the field, with the live preview; `pos` is the character position in the template.

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `token.unclosed` | error | GUI | `pos` | A `{` has no matching `}`. Use `{{` for a literal brace. |
| `token.unexpected_close` | error | GUI | `pos` | A `}` without `{`. Use `}}` for a literal brace. |
| `token.unknown` | error | GUI | `pos`, `name` | Unknown token name. |
| `token.format_invalid` | error | GUI | `pos`, `format` | Unknown format letters in the token. |
| `token.offset_invalid` | error | GUI | `pos`, `offset` | Offset not understood (e.g. `-1d`, `+2w`, `@month-start`). |
| `token.empty` | error | GUI | `pos` | `{}` with nothing inside. |

## Run time (scheduled runs and "send now")

Collected per run: one Toast (and one message to each error recipient, if any) lists all the problems of
that run, per file and recipient.

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `file.not_found` | skip | Toast, Err, Status | `schedule`, `path` | No file matches the pattern now. Tried again at the next slot. |
| `file.multiple_matches` | skip | Toast, Err, Status | `schedule`, `path`, `count` | The glob matched several files and the spec is in "error" mode. Choose "newest" or "all", or make the pattern stricter. |
| `file.locked` | skip | Toast, Err, Status | `schedule`, `file` | The file is locked by another program (e.g. being saved). |
| `file.unreadable` | skip | Toast, Err, Status | `schedule`, `file` | Access denied or read error. |
| `xlsx.open_failed` | skip | Toast, Err, Status | `schedule`, `file` | Not a valid workbook, or corrupted. |
| `xlsx.sheet_missing` | skip | Toast, Err, Status | `schedule`, `file`, `sheet` | A requested sheet doesn't exist in the file. The file is not sent. |
| `xlsx.no_cached_values` | warning | Toast, Err, Status | `schedule`, `file`, `sheet`, `count` | Sent, but `count` formula cells had no cached value (file generated by a script, not saved by Excel): they are blank. |
| `xlsx.process_failed` | skip | Toast, Err, Status | `schedule`, `file` | Reducing the workbook failed (temp copy, save). |
| `temp.unavailable` | skip | Toast, Err, Status | `schedule` | The temp folder can't be written. |
| `send.not_connected` | skip | Toast, Err, Tray, Status | `schedule`, `channel`, `recipient` | The recipient's channel is not connected at slot time. Err only reaches error recipients on other, connected channels. |
| `send.too_large` | skip | Toast, Err, Status | `schedule`, `file`, `channel`, `size`, `max` | The file is above the channel's size limit; skipped for that recipient only. |
| `send.caption_too_long` | skip | Toast, Err, Status | `schedule`, `channel`, `max` | The caption is longer than the channel allows; skipped for that recipient only. |
| `send.files_unsupported` | skip | Toast, Err, Status | `schedule`, `channel` | The channel can't send files (text-only provider). |
| `send.upload_failed` | skip | Toast, Err, Status | `schedule`, `file`, `recipient` | Upload failed after one short retry. |
| `send.failed` | skip | Toast, Err, Status | `schedule`, `file`, `recipient` | The service refused or didn't acknowledge the message after one short retry. |
| `send.recipient_invalid` | skip | Toast, Err, Status | `schedule`, `recipient` | The service rejected the recipient (number deactivated, removed from the group, chat deleted…). |

## Skip reasons (status and log)

Not errors: shown in the schedule's last result and in the log, never as a Toast.

| Code | Level | Where | Params | Meaning |
| --- | --- | --- | --- | --- |
| `slot.missed` | skip | Status | `schedule`, `slot` | The slot passed while the PC was asleep/off or the app was closed; never recovered. |
| `slot.late` | skip | Status | `schedule`, `slot` | The run would start more than 5 minutes after the slot; skipped. |
| `slot.dst_gap` | skip | Status | `schedule`, `slot` | The slot falls in the hour skipped by the spring DST change. |
| `slot.dst_repeat` | skip | Status | `schedule`, `slot` | Second occurrence of a slot in the hour repeated by the autumn DST change. |
| `slot.paused` | skip | Status | `schedule`, `slot` | The routine was paused from the tray. |

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
| `channel.targets_failed` | error | GUI | `channel` | The list of groups/chats could not be loaded. |
| `channel.secret_unavailable` | error | GUI, Toast | `channel` | The channel's stored secret can't be decrypted (e.g. Windows user profile changed). Enter it again. |

## WhatsApp provider

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `wa.number_invalid` | error | GUI | `value` | Not a valid international number (needs the country prefix). |
| `wa.not_on_whatsapp` | error | GUI | `value` | The number has no WhatsApp account. |
| `wa.group_not_found` | error | GUI, Toast, Err | `channel`, `name` | The group is no longer among the account's groups (left or removed). At run time that recipient is skipped. |
| `wa.client_outdated` | error | Toast, Tray, GUI | `channel` | WhatsApp rejected this version of whatsmeow. An update check starts; install the update when offered. |
| `wa.logged_out` | error | Toast, Tray, GUI | `channel` | The linked device was removed from the phone or expired. Pair again from the Channels page. |
| `wa.pair_timeout` | error | GUI | `channel` | QR codes expired without a scan. Start again. |
| `wa.pair_failed` | error | GUI | `channel` | Pairing refused or interrupted. |
| `wa.session_corrupt` | error | GUI, Toast | `channel` | The channel's `session.db` can't be opened. It is renamed to `session.db.corrupt-<timestamp>` in the channel dir and a new pairing is needed. |

## Error recipients

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `notify.failed` | warning | Toast, Status | `recipient` | Sending the error report to an error recipient failed (the Toast is still shown). |

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
