# wchron

Sends files at scheduled times to WhatsApp numbers and groups (other channels such as Telegram or ntfy
can come later), from your Windows PC.

wchron sits in the system tray and, at the times you set, picks the right file (for example today's
report, found by a name with the date in it), optionally reduces an Excel workbook to the sheets you
choose, and sends it to one or more recipients, for example as a document through your WhatsApp account,
linked like WhatsApp Web. You can connect several accounts (channels), also of the same service.

> **Status: under development.** This README describes the planned behaviour.

> **Risk: unofficial client.** wchron uses [whatsmeow](https://github.com/tulir/whatsmeow), an
> unofficial WhatsApp client. Using it goes against WhatsApp's terms of service, and the account could in
> theory be restricted. wchron is meant for low volume (one file per hour or less) to recipients who
> expect it. Use it at your own risk.

## What it does and doesn't do

- Sends files on a schedule to one or more recipients per schedule, one file after the other with a short
  pause, with an optional caption.
- Never sends error messages to the recipients: problems are shown as desktop notifications and,
  optionally, sent to one or more "error recipients" (for example your own WhatsApp number).
- Doesn't read or answer messages, doesn't send in bulk, doesn't embed WhatsApp Web.
- Windows 10/11 only, for now. Italian and English. Light, dark or follow Windows.

## Install

Download `wchron-amd64-installer.exe` from the latest [release](https://github.com/jacopofilonzi/wchron/releases)
and run it. It installs for the current user only (`%LOCALAPPDATA%\Programs\wchron`), without
administrator rights. wchron starts at login, minimized to the tray; launching it again just opens the
window.

On first start, open **Channels** in the window, add a WhatsApp channel and scan the QR code from your phone
(Settings → Linked devices → Link a device).

## Schedules

Each schedule has one or more recipients (on one or more channels), one or more files and a cadence:

- **Interval**: every N hours and minutes, counted from a start time (default 00:00) and restarting each
  day, so 1h30 from 08:00 means 08:00, 09:30, 11:00… Optionally only inside a time window
  (e.g. 08:00–23:00).
- **Fixed times**: a list of times, e.g. 09:00, 13:30, 18:00.
- **Days**: any days of the week.

The editor shows the next 5 runs. Each send starts at a random moment in the first 2 minutes after the
scheduled time.

**Missed runs are not recovered.** If the PC was asleep or off, the app was closed or the routine was
paused, that run is skipped and noted in the status page; wchron never sends a burst to catch up. A run
that would start more than 5 minutes late is skipped too.

Daylight saving time follows the Windows clock: a run in the hour skipped in spring doesn't happen; a run
in the hour repeated in autumn happens once.

**Recipients**: each one is a channel plus a target. On WhatsApp: a number with the international prefix
(checked to be on WhatsApp when you save), or one of
your groups, picked from a list. Renaming the group doesn't break the schedule.

**Deleting** a schedule or a channel is permanent: wchron asks you to confirm first.

## Files and dates in names

A file path can contain date tokens, filled in at send time, and wildcards (`*`, `?`) in the file name.
The same tokens work in captions and in Excel sheet names. The editor shows what the text becomes right
now, and points at the exact character when something is wrong.

### Tokens

A token is written in braces: `{date}`, optionally with modifiers that move the date and a format after
a colon:

```text
{date<modifiers>:<format>}
```

Everything outside braces is copied as it is. Write `{{` for a literal `{` and `}}` for a literal `}`.
The date is the one of the scheduled send (the slot time, in the PC's time zone), not the moment the
file happens to be processed.

**Modifiers** come right after `date`, as many as you like, applied left to right:

| Modifier | Meaning |
| --- | --- |
| `+N` / `-N` with `d`, `w`, `m`, `y` | N days, weeks, months or years later / earlier (`-1d`, `+2w`, `-1m`, `+1y`; N up to 4 digits). Months and years stop at the last day of the month: 31 January `+1m` is 28 (or 29) February. |
| `@month-start` / `@month-end` | first / last day of the month |
| `@week-start` / `@week-end` | Monday / Sunday of the week |
| `@year-start` / `@year-end` | 1 January / 31 December |

Order matters: `{date@month-start-1d}` is the last day of the previous month, `{date-1d@month-start}`
the first day of yesterday's month. Modifiers change only the date: hours, minutes and seconds stay those
of the send time.

**Format** (after `:`; without one it is `YYYY-MM-DD`). Names follow the app language:

| Letters | Meaning | Example (it / en) |
| --- | --- | --- |
| `YYYY` | year | `2026` |
| `YY` | year, 2 digits | `26` |
| `MMMM` | month name | `ottobre` / `October` |
| `MMM` | short month name | `ott` / `Oct` |
| `MM` | month, 2 digits | `10` |
| `M` | month | `10` |
| `DD` | day, 2 digits | `26` (the 5th: `05`) |
| `D` | day | `26` (the 5th: `5`) |
| `dddd` | weekday name | `lunedì` / `Monday` |
| `ddd` | short weekday name | `lun` / `Mon` |
| `HH` | hour (00–23), 2 digits | `09` |
| `H` | hour (0–23) | `9` |
| `mm` | minutes, 2 digits | `05` |
| `ss` | seconds, 2 digits | `07` |

The longest code wins (`MMMM` is the month name, not `MM` twice). Any other Latin letter is an error;
everything else (`-`, `_`, `.`, `/`, spaces, digits, accented letters…) is copied as it is. Italian names
are lowercase (`lunedì`, `gennaio`), English ones capitalized (`Monday`, `January`). Remember that `:` and
`/` are not allowed in Windows file names: use them in captions only.

### Examples

For a send on Monday 26 October 2026 at 09:05:07, with the app in Italian (English in brackets where it
differs):

| Template | Result |
| --- | --- |
| `{date}` | `2026-10-26` |
| `{date:DD_MM_YYYY}` | `26_10_2026` |
| `{date-1d:DD_MM_YYYY}` | `25_10_2026` (yesterday) |
| `{date:dddd D MMMM}` | `lunedì 26 ottobre` (`Monday 26 October`) |
| `{date@month-start-1m:MMMM YYYY}` | `settembre 2026` (`September 2026`): last month |
| `{date@month-start:DD-MM-YYYY}` | `01-10-2026` |
| `{date@month-start-1d:DD-MM-YYYY}` | `30-09-2026`: last day of last month |
| `{date-1w@week-start:DD/MM}` – `{date-1w@week-end:DD/MM}` | `19/10` – `25/10`: last week |
| `{date@year-end:YYYY-MM-DD}` | `2026-12-31` |
| `Report {date:HH.mm}` | `Report 09.05` |
| `C:\Reports\vendite_{date:YYYYMMDD}.xlsx` | `C:\Reports\vendite_20261026.xlsx` |
| `{{date}} = {date}` | `{date} = 2026-10-26` |

If a template is wrong, the editor tells you what and where (the character position), e.g. an unclosed
`{`, an unknown format letter or modifier; [ERRORS.md](ERRORS.md) lists the `token.*` messages.

### Wildcards

If a wildcard matches several files, by default nothing is sent and you get a notification; for each file
you can choose to send the newest match or all matches instead.

### Excel: send only some sheets

For `.xlsx`/`.xlsm` files you can list the sheets to keep (names can contain tokens). wchron works on a
copy, never on your file: it turns the formulas of those sheets into values, removes the other sheets and
sends the copy.

- wchron doesn't recalculate formulas: it uses the values Excel saved in the file. Files saved by Excel
  are fine; files generated by scripts may have no saved values, and wchron warns you when that happens.
- If a sheet doesn't exist, that file is not sent and you get a notification.
- Charts, images, conditional formatting and data validation may not survive the removal of other
  sheets. Known limitations will be listed here.

## When something goes wrong

A missing, locked or unreadable file, a missing sheet or a failed send: that file is skipped, the other
files and recipients still get theirs, and at the end one desktop notification (and, if set, one message to each
error recipient) lists the problems. The next scheduled run simply tries again. Every problem
has a code; [ERRORS.md](ERRORS.md) lists them all.

If a channel disconnects, the tray icon shows it and wchron reconnects by itself; if a WhatsApp linked device was
removed, the window asks you to scan the QR again.

## Tray

- **Activate routine**: pause or resume all schedules (a pause is remembered across restarts).
- **Keep awake**: keeps the PC from sleeping while wchron runs (the screen can still turn off). It
  starts off at every launch, unless "Keep the PC awake at startup" is on in Settings.
- Open the window, see the connection state and last send, quit.

The icon shows whether the routine is active, paused, sending, in error or with a channel disconnected,
and whether keep awake is on.

## Updates

wchron checks for a new release at startup, and also when a channel such as WhatsApp rejects the connection (an update may
be needed). It only notifies you: nothing is installed until you click **Update**. Then it downloads the
installer, waits until no send is in progress and the next one is at least 10 minutes away, closes,
updates and restarts by itself.

Before starting a new version, wchron backs up your data (`%APPDATA%\wchron\backups\`, one folder per
upgrade named after the date and the previous version; the 10 most recent are kept); if the backup
fails, it doesn't start. Your schedules, settings and channels (including the WhatsApp link) are kept across updates.

### Upgrade notes

None yet. Anything you need to do before installing a specific version will be listed here.

## Your data

Everything is in `%APPDATA%\wchron`: `config.json` (schedules, channels, settings), `state.json` (last runs,
whether the routine is paused), `channels\` (the WhatsApp link and other channel credentials: keep it
private), `logs\` and `backups\`.

If wchron can't read `config.json` (for example after a hand edit), or the file was saved by a newer
version, it shows an error and doesn't start, leaving the file as it is: fix it, restore it from
`backups\` or install the newer version. If `state.json` is damaged, wchron starts anyway with the
last-run information reset, and keeps the damaged file next to it as `state.json.corrupt-<date>`.

## Development

See [AGENTS.md](AGENTS.md) and [STRUCTURE.md](STRUCTURE.md). `make help` lists the commands.
