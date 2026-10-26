# Error codes

Every error, warning and skip reason the user can see. The code is stable (never renamed once released);
the text the user reads is `error.<code>` in `locales/it.json` / `locales/en.json`, filled with the params.
Logs always carry the code plus an English message.

Rules:

- Codes are dotted lowercase, `area.reason`. The registry is `internal/apperr/codes.go`; a test fails if
  the registry, both catalogs and this file disagree.
- **None of these is ever sent to a schedule's recipient.** Where a row says "WA", the text goes only to
  the error notification number, if set.
- Where: `GUI` inline in the window (form or dialog), `Toast` Windows desktop notification, `WA` error
  notification number, `Status` status page and log viewer, `Tray` tray icon/status line.
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
| `recipient.required` | error | GUI | — | No recipient chosen. |
| `recipient.number_invalid` | error | GUI | `value` | Not a valid international number (needs the country prefix). |
| `recipient.not_on_whatsapp` | error | GUI | `value` | The number has no WhatsApp account. |
| `recipient.check_unavailable` | error | GUI | — | The number can't be checked now (WhatsApp not connected). Connect and save again. |
| `recipient.group_not_found` | error | GUI, Toast, WA | `name` | The group is no longer in the account's groups (left or removed). At run time the schedule is skipped. |
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

Collected per run: one Toast (and one WA message, if the error number is set) lists all the problems of
that run.

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `file.not_found` | skip | Toast, WA, Status | `schedule`, `path` | No file matches the pattern now. Tried again at the next slot. |
| `file.multiple_matches` | skip | Toast, WA, Status | `schedule`, `path`, `count` | The glob matched several files and the spec is in "error" mode. Choose "newest" or "all", or make the pattern stricter. |
| `file.locked` | skip | Toast, WA, Status | `schedule`, `file` | The file is locked by another program (e.g. being saved). |
| `file.unreadable` | skip | Toast, WA, Status | `schedule`, `file` | Access denied or read error. |
| `file.too_large` | skip | Toast, WA, Status | `schedule`, `file`, `size` | Above WhatsApp's document size limit. |
| `xlsx.open_failed` | skip | Toast, WA, Status | `schedule`, `file` | Not a valid workbook, or corrupted. |
| `xlsx.sheet_missing` | skip | Toast, WA, Status | `schedule`, `file`, `sheet` | A requested sheet doesn't exist in the file. The file is not sent. |
| `xlsx.no_cached_values` | warning | Toast, WA, Status | `schedule`, `file`, `sheet`, `count` | Sent, but `count` formula cells had no cached value (file generated by a script, not saved by Excel): they are blank. |
| `xlsx.process_failed` | skip | Toast, WA, Status | `schedule`, `file` | Reducing the workbook failed (temp copy, save). |
| `temp.unavailable` | skip | Toast, WA, Status | `schedule` | The temp folder can't be written. |
| `send.not_connected` | skip | Toast, Tray, Status | `schedule` | WhatsApp is not connected at slot time. Not sent over WA (it's the same session). |
| `send.upload_failed` | skip | Toast, WA, Status | `schedule`, `file` | Upload to WhatsApp failed after one short retry. |
| `send.failed` | skip | Toast, WA, Status | `schedule`, `file` | WhatsApp refused or didn't acknowledge the message after one short retry. |
| `send.recipient_invalid` | skip | Toast, WA, Status | `schedule` | WhatsApp rejected the recipient (number deactivated, removed from the group…). |

## Skip reasons (status and log)

Not errors: shown in the schedule's last result and in the log, never as a Toast.

| Code | Level | Where | Params | Meaning |
| --- | --- | --- | --- | --- |
| `slot.missed` | skip | Status | `schedule`, `slot` | The slot passed while the PC was asleep/off or the app was closed; never recovered. |
| `slot.late` | skip | Status | `schedule`, `slot` | The run would start more than 5 minutes after the slot; skipped. |
| `slot.dst_gap` | skip | Status | `schedule`, `slot` | The slot falls in the hour skipped by the spring DST change. |
| `slot.dst_repeat` | skip | Status | `schedule`, `slot` | Second occurrence of a slot in the hour repeated by the autumn DST change. |
| `slot.paused` | skip | Status | `schedule`, `slot` | The routine was paused from the tray. |

## WhatsApp session

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `wa.client_outdated` | error | Toast, Tray, GUI | — | WhatsApp rejected this version of the app. An update check starts; install the update when offered. |
| `wa.disconnected` | error | Toast, Tray, Status | — | Connection lost; the app reconnects by itself with backoff. |
| `wa.logged_out` | error | Toast, Tray, GUI | — | The linked device was removed from the phone or expired. Pair again from the WhatsApp page. |
| `wa.pair_timeout` | error | GUI | — | QR codes expired without a scan. Start again. |
| `wa.pair_failed` | error | GUI | — | Pairing refused or interrupted. |
| `wa.session_corrupt` | error | GUI, Toast | — | `session.db` can't be opened. It is moved to the trash folder and a new pairing is needed. |
| `wa.groups_failed` | error | GUI | — | The group list could not be loaded. |
| `wa.not_connected` | error | GUI | — | An action that needs the session (group list, number check, send now) while disconnected. |

## Error notification number

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `notify.number_invalid` | error | GUI | `value` | Not a valid international number. |
| `notify.number_not_on_whatsapp` | error | GUI | `value` | The number has no WhatsApp account. |
| `notify.wa_failed` | warning | Toast, Status | — | Sending the error report to the error number failed (the Toast is still shown). |

## Platform

| Code | Level | Where | Params | Meaning / what to do |
| --- | --- | --- | --- | --- |
| `autostart.failed` | error | GUI | — | Couldn't enable/disable start at login (registry write refused). |
| `keepawake.failed` | error | Tray, Toast | — | Windows refused to keep the PC awake. |
| `trash.not_found` | error | GUI | `id` | The item is no longer in the trash. |
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
