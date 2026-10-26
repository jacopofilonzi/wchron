# wchron

Sends files at scheduled times to WhatsApp numbers and groups (other channels such as Telegram or ntfy
can come later), from your Windows PC.

wchron sits in the system tray and, at the times you set, picks the right file (for example today's
report, found by a name with the date in it), optionally turns an Excel workbook into a values-only copy
with just the sheets you choose, and sends it to one or more recipients, for example as a document through your WhatsApp account,
linked like WhatsApp Web. You can connect several accounts (channels), also of the same service.

> **Status: under development.** Channels, schedules, files, Excel processing and the status page work;
> the hardening for release and the automatic updates (described below) are still to come.

> **Risk: unofficial client.** wchron uses [whatsmeow](https://github.com/tulir/whatsmeow), an
> unofficial WhatsApp client. Using it goes against WhatsApp's terms of service, and the account could in
> theory be restricted. wchron is meant for low volume (one file per hour or less) to recipients who
> expect it. Use it at your own risk.

## What it does and doesn't do

- Sends files on a schedule to one or more recipients per schedule, one file after the other with a short
  pause (2–5 s), with an optional caption.
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

## Channels

A channel is one account on a service: today WhatsApp, linked to your phone like WhatsApp Web. You can add
several, also of the same service (e.g. a personal and a work number). The **Channels** page lists them
with their state (connected, connecting, not connected, not paired, logged out, update needed).

- **Add**: **Add channel**, choose the service and a name, then **Create and set up**. For WhatsApp a QR code
  appears: on the phone open WhatsApp → Settings → Linked devices → Link a device and point it at the
  code. The code changes every few seconds; after about two minutes without a scan the pairing stops and
  you can start it again. On the phone the device shows as **wchron**. A paired channel shows its
  account with only the last 3 digits.
- **Log out** removes the linked device from the account (it disappears from the phone's list) and keeps
  the channel, ready for a new pairing; nothing is sent through it until then. **Pair again** does a log
  out and a new pairing in one step. Both need the channel to be connected.
- **Rename** changes only the label.
- **Delete** asks you to confirm and shows which schedules lose recipients; a schedule left without any
  recipient is disabled. If the channel is connected, its linked device is removed from the phone first;
  if it isn't, remove "wchron" by hand from the phone's Linked devices.

A channel that loses the connection reconnects by itself (after 5 seconds, then waiting longer each
time, up to 5 minutes). It doesn't when the phone removed the linked device (pair it again), when
WhatsApp rejects the app version (update wchron), when the same session was opened by another copy of
wchron, or after a temporary block from WhatsApp (restart wchron later).

Pairing, log out and delete act on your real WhatsApp account: try them first with a secondary number.

## Schedules

The **Schedules** page lists them with when they run, their recipients, the next send and how the last one
went (sent, sent in part, not sent or skipped, with the first problem). The switch on each row turns a
schedule on or off at once (one that can't run, e.g. left without recipients, can't be turned on); the
icons duplicate it (the copy is off and named "… (copy)") or delete it, after a confirmation that names it.
**New schedule** and a click on a name open the editor, a page of its own:

- **Name** and, on the right, **Active**.
- **When**: interval or fixed times, start time, optional time range, days (with the **Weekdays** and
  **All** shortcuts). The right column shows the next 5 sends as you type.
- **To whom**: **Add recipient**, choose the channel (when there are several), then a number or a group
  (see Recipients below). Each recipient shows its channel and whether it was verified.
- **Caption**: optional, with the text it becomes right now.
- **Files, in this order**: add, reorder with the arrows, remove; for each one the path (with what it
  matches right now), what to do when several files match, the processing (only the ones that fit the
  file type), an own caption and the name to send (with the names the recipients will see).

Nothing is saved until **Save**: wchron then checks everything and marks the fields to fix (an empty name,
no recipient or file, a wrong time, a path that is not complete, a date token with a typo…). Leaving the
editor with unsaved changes asks first. Removing a recipient or a file that was already saved asks for a
confirmation; it is gone for good when you save. **Send now** and **Delete schedule** are in the right
column; send now needs the changes saved first and asks before sending.

Each schedule has one or more recipients (on one or more channels), one or more files and a cadence:

- **Interval**: every N hours and minutes, counted from a start time (default 00:00) and restarting each
  day, so 1h30 from 08:00 means 08:00, 09:30, 11:00… up to the last one before midnight (from 1 minute
  to 24 hours). Optionally only inside a time window, both ends included: every hour from 08:00 to
  20:00 also sends at 20:00. A window can cross midnight (e.g. 22:00–02:00): the part after midnight
  belongs to the day the window opened, so a window on Mondays only also sends early on Tuesday, and
  not early on Monday.
- **Fixed times**: a list of times, e.g. 09:00, 13:30, 18:00.
- **Days**: any days of the week.

The editor shows the next 5 runs. Each send starts at a random moment in the first 2 minutes after the
scheduled time (the times themselves never drift). Different schedules run at the same time without
waiting for each other; a schedule never overlaps itself: if its previous run is still going when the
next time comes, that time is skipped and noted.

**Missed runs are not recovered.** If the PC was asleep, the clock jumped or the routine was paused,
those runs are skipped and noted in the status page as one summary per schedule ("skipped sends: 12,
from 08:15 to 11:00"), not one line per run; wchron never sends a burst to catch up. A run that would
start more than 5 minutes late is skipped too. Times that passed while wchron was closed are simply not
counted. A schedule with an invalid cadence (e.g. after editing `config.json` by hand) is ignored and
logged, the others keep running.

Daylight saving time follows the Windows clock: a run in the hour skipped in spring doesn't happen; a run
in the hour repeated in autumn happens once.

**Recipients**: each one is a channel plus a target. On WhatsApp: a number with the international prefix
(spaces and dashes are fine; checked to be on WhatsApp when you add it), or one of your groups, searched
by name in the list of your groups (this needs the channel connected). Renaming the group doesn't break the schedule. If the channel is not connected when you add a number, the
recipient is saved as **not verified** and checked automatically the next time the channel connects; one
that fails the check (no WhatsApp account, group left) is marked and never receives anything until you fix it.

**Deleting** a schedule or a channel is permanent: wchron asks you to confirm first.

### What happens at a run

1. Every file of the schedule is found (dates in the name are computed for the scheduled time, not for the
   moment the send actually starts) and copied once to a temporary folder (`%TEMP%\wchron`): your
   files are never modified, and a file that is being written at that moment is sent as it is at that
   moment (wchron doesn't wait for it). The copies are deleted at the end, also when something fails.
2. If, right before the first send, more than 5 minutes have passed since the scheduled time, the run is
   skipped.
3. Each recipient gets all the files, in order, one after the other. Recipients on different channels
   are served at the same time; on the same channel one message at a time, with a random pause of 2 to 5
   seconds between two messages (also between different schedules sending on that channel).
4. A recipient not verified yet is checked right before its first file (if its channel is connected);
   one that fails the check is skipped and marked. A file larger than the channel allows, or a caption too
   long for it, is skipped for that recipient only. A recipient whose channel is not connected is skipped.

**Captions**: the schedule caption goes on the first file each recipient receives; the other files go
without a caption, unless a file has its own caption, which it always carries (an empty one too).

**Name to send**: each file can have its own name for the recipients, with date tokens (e.g.
`Vendite {date:DD-MM-YYYY}`); empty keeps the file's name. If the name has no extension, the file's one is
added (`Report {date:DD.MM.YYYY}` becomes `Report 26.10.2026.xlsx`: the date's dots don't count as an
extension). Characters Windows doesn't allow in file names (`< > : " /  | ? *`), a dot or space at the
end, and reserved names such as `CON` are refused when you save; if a date makes the name invalid at send
time (e.g. a `/` in the format), that file is skipped and you get a notification. When a file's path matches
several files and you send them all, a name to send is numbered in name order, so they don't all arrive with
the same name: `Report (1).xlsx`, `Report (2).xlsx`; a single match gets no number, and without a name to
send the files keep their own (different) names. The editor shows the names the recipients will see.

**Send now** runs a schedule immediately, whatever its days, time window or tolerance (not while that
schedule is already sending). Its problems show only as a desktop notification, never as a message to the
error recipients.

## Files and dates in names

A file path can contain date tokens, filled in at send time, and wildcards (`*`, `?`) in the file name.
The same tokens work in captions, in names to send and in Excel sheet names. The editor shows what the text becomes right
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

In the file name (never in the folders) `*` stands for any text and `?` for one character, e.g.
`C:\Reports\vendite_*.pdf`; upper and lower case don't matter, and `[` `]` are ordinary characters. Paths
must be full (`C:\…` or `\\server\share\…`).

If a wildcard matches several files, by default nothing is sent and you get a notification; for each file
you can choose to send the newest match (latest modified) or all matches (in name order) instead (with a
name to send, they are numbered: see "Name to send"). If
nothing matches, that file is skipped and tried again at the next run.

### Excel: original or values only

For Excel files you choose how to send them:

- **Original**: the file as it is (like any other file).
- **Values only**: a copy without formulas or macros, with only the sheets you choose. It works for
  `.xlsx`, `.xlsm`, `.xltx` and `.xltm` files (not the old `.xls` or `.xlsb`) and is always sent as
  `.xlsx` (`report.xlsm` arrives as `report.xlsx`, unless you set a name to send).

With **values only**, wchron works on a copy, never on your file:

- Every formula becomes the value Excel saved for it: numbers, dates (with their format), text, TRUE/FALSE
  and error values such as `#DIV/0!` stay what they were, with the cell's formatting. **Formulas are not
  recalculated**: files saved by Excel are fine; files generated by scripts may have no saved values, and
  then those cells arrive empty and you get a warning with how many there were. Values taken from sheets
  that are not sent are kept.
- **Sheets to include**: type a name and press Enter for each one (dates allowed, e.g. `{date:MMMM YYYY}`:
  the editor shows what it becomes today); upper and lower case and spaces around the name don't matter.
  Empty list: all sheets. The sheets keep the workbook's order. When the file exists now, **Read the
  sheets from the file** lists them: click one to include it or leave it out.
- A listed sheet that isn't in the file: the file is sent anyway with the other sheets, and you get a
  warning (on the desktop and, if set, to the error recipients). If none of the listed sheets is there,
  the file is not sent.
- Macros (VBA, and old Excel 4 macro sheets) are removed. Hidden sheets you include stay hidden; if all
  the included sheets are hidden, the first one is shown (Excel needs one).
- Names (named ranges) that point at a removed sheet are removed.
- Password-protected (encrypted) workbooks can't be processed: send them as **Original**.

Known limitations of **values only**: charts, pivot tables, conditional formatting and data validation
that use data from a removed sheet lose it (Excel may show them empty or offer to repair the file);
pivot tables are not refreshed; links to other workbooks keep their last values but stay links; dynamic
array formulas keep their values but no longer spill. Images, shapes, comments and formatting of the
included sheets are normally kept.

## Status and log

The **Status and log** page shows the channels (how many are connected), today's runs and files sent, the
runs with problems in the last 7 days and whether keep awake is on; then the last 50 runs and skips, each
with its schedule, how many sends went out and the first problem; then the log, filtered by level (all,
warnings, errors), with **Open folder** for the log files. The runs are read from the log, so they go back
as far as the log does (about 25 MB). Opening the page clears the tray's error state.

## When something goes wrong

A missing, locked or unreadable file, an Excel file without any of its sheets or a failed send: that file is skipped, the other
files and recipients still get theirs, and at the end one desktop notification (and, if set, one message to each
error recipient) lists the problems. The error recipients are set in **Settings → Error recipients**, on
any channel, like a schedule's recipients (removing one asks first and takes effect at once). The next scheduled run simply tries again. Every problem
has a code; [ERRORS.md](ERRORS.md) lists them all.

Notifications are not repeated at every run: the first failure of a schedule is reported at once; while
the same problems persist they are reported again at most every 6 hours; different problems are reported
at once. When the schedule works again after a reported failure, a "works again" notice is sent the same
way. A run that goes fine sends no notification: its result is in the Status page and the tray. Clicking a
notification opens the Status page. If the message to an error recipient can't be sent, a second desktop
notification says so.

If a channel disconnects, the tray icon shows it and wchron reconnects by itself; if a WhatsApp linked device was
removed, the window asks you to scan the QR again.

## Tray

- **Activate routine**: pause or resume all schedules (a pause is remembered across restarts).
- **Keep awake**: keeps the PC from sleeping while wchron runs (the screen can still turn off). It
  starts off at every launch, unless "Keep the PC awake at startup" is on in Settings.
- Open the window, see the connection state and the next scheduled send ("Next: Report vendite
  14:00", with the day when it's not today), quit.

The icon shows whether the routine is active, paused, sending, in error or with a channel disconnected,
and whether keep awake is on. The error state stays until the failing schedule works again or you open
the Status page.

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
private), `logs\` and `backups\`. **Settings → Data** shows the folder and the date of the last backup, and opens
the folder.

If wchron can't read `config.json` (for example after a hand edit), or the file was saved by a newer
version, it shows an error and doesn't start, leaving the file as it is: fix it, restore it from
`backups\` or install the newer version. If `state.json` is damaged, wchron starts anyway with the
last-run information reset, and keeps the damaged file next to it as `state.json.corrupt-<date>`.

## Development

See [AGENTS.md](AGENTS.md) and [STRUCTURE.md](STRUCTURE.md). `make help` lists the commands.
