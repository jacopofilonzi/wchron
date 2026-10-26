# Project structure

wchron is a Windows tray app for one user: at scheduled times it resolves file paths (with date tokens),
optionally turns Excel workbooks into values-only copies of selected sheets, and sends the files to one or more recipients on
one or more channels. Delivery goes through provider plug-ins: WhatsApp (whatsmeow linked device) first,
Telegram/ntfy possible later without structural changes. One Go process holds the core (scheduler,
pipeline, channels, state); the Svelte GUI in a Wails window talks to it through generated bindings and events.

> Status: milestones 1 (skeleton), 2 (data, logging), 3 (tokens), 4 (scheduler), 5 (providers, WhatsApp),
> 6 (pipeline, resolver, passthrough, notifications), 7 (values-only Excel processor, send names) and 8 (GUI:
> schedules list and editor, recipients, previews, status and log, error recipients, e2e) are in place; the
> rest of this file is the planned layout.
> Packages and files that exist today are marked **(m1)** … **(m8)**. Update this file as each milestone lands.

## Milestones

- [x] 1. Skeleton: Wails 3 + Svelte + Tailwind 4, tray with the two toggles and dynamic icon, single
      instance, autostart, Makefile, i18n catalog, version script.
- [x] 2. Data dir, config, state, migrations + backup, logging (rotating files, reader, masking), platform
      paths and fatal dialog, error registry.
- [x] 3. Token engine with tests (Italian names, offsets, escaping).
- [x] 4. Scheduler with tests (interval, fixed times, window, days, jitter, skipped slots, DST both ways, fake clock).
- [x] 5. Provider layer (interface, registry, channel manager, fake provider) + WhatsApp provider: QR
      pairing, session persistence, number check, group list, document send; minimal Channels page.
      DPAPI secrets are deferred to the first provider that needs one (Telegram/ntfy).
- [x] 6. Pipeline with `passthrough`, resolver, multiple files × multiple recipients, pauses, error
      aggregation, notifications to the error recipients (6 h repeat, "works again"), send now, toasts.
- [x] 7. `xlsx-snapshot` processor ("values only": no formulas, no macros, chosen sheets, .xlsx) with tests
      on synthetic workbooks (formulas, missing sheet, locked file, no cached values); `send_name` for every
      file spec; `ReadSheets`.
- [x] 8. GUI screens: schedules list and editor (cadence, recipients with the provider's picker, caption,
      files with previews, Excel options, send names numbered for "all"), status and log, error recipients,
      data folder; schedule bindings with field-addressed validation; Playwright e2e on a fake backend.
- [ ] 9. Hardening: reconnection, installer, release workflow (log rotation landed with milestone 2).
- [ ] 10. Update notification and self-update.

```text
wchron/
├── main.go             wiring (m1, m2): Wails app + single instance first, then devenv → paths → log file →
│                       startup.Open (config/state load, backup, migrate) → store → service, window, tray;
│                       (m4) scheduler reloaded and started on ApplicationStarted, stopped first on quit;
│                       (m5) provider registry (the only place naming the providers) + channel manager, started
│                       after the scheduler on ApplicationStarted, closed after it on quit;
│                       (m6) toaster, notifier, processor registry (the only place naming the processors),
│                       pipeline (the scheduler's Runner; OnStart/OnFinish → Store + run events); on
│                       ApplicationStarted: toasts (main thread), temp cleanup, scheduler, channels;
│                       (m7) xlsxsnapshot registered next to passthrough, its ReadSheets given to the service;
│                       (m8) app.Configure: processors, data/backups dirs, the schedule-change hook (scheduler
│                       Reload + schedules:changed, also after a channel delete), platform.OpenFolder
├── internal/           Go (go.mod: 1.26, toolchain 1.27) — core and platform layer (see below)
├── locales/            it.json, en.json (shared i18n catalog, flat namespaced keys) + embed.go (m1)
├── frontend/           Svelte 5 + TS + Vite + Tailwind 4 — GUI
├── build/              Wails build assets (m1): config.yml, Taskfile.yml (frontend, bindings), appicon.png
│                       (1024 px logo), windows/ (Taskfile.yml, icon.ico, wails.exe.manifest, info.json, nsis/
│                       per-user installer). icon.ico and appicon.png are written by `make icons`
├── scripts/            set-version.mjs (m1)
├── .github/workflows/  release.yml (on a v* tag only: build + release)
├── update.config.json  build-time update settings, embedded in the binary
├── Taskfile.yml        Wails tasks (build, package, run, dev), wrapped by the Makefile (m1)
├── .env.example        dev-only overrides; `.env` is git-ignored
├── Makefile            entry point for every command (`make help`, see below) (m1)
├── README.md           user docs: behaviour, install, updates, ToS risk
├── ERRORS.md           every user-visible error code
└── bin/                build output: wchron.exe, wchron-amd64-installer.exe (git-ignored)
```

## Backend (`internal/`)

```text
internal/
├── app/            (m1, m2) Store (state loaded from config/state, persisted on change, side effects,
│                   subscribers) + Service (the Wails bindings) + events; (m5) the Store is the channel
│                   manager's ConfigStore and holds the channel list; channel bindings and events;
│                   (m6) runs in progress / failing schedules (Snapshot sending, run_error), SendNow,
│                   AckErrors, run events; (m8) schedules.go (list, get, save, duplicate, delete, enable,
│                   previews), validate.go (field-addressed problems, design in its doc), settings.go (error
│                   recipients, folders, last backup), history.go (run history read from the log)
├── apperr/         (m1) Error{Code, Params, Err}, codes.go (registry mirrored by ERRORS.md), JSON `{code, params}`
├── clock/          (m3) Clock interface (Now), Real, Fake (Set/Advance) for tests
├── config/         (m2) config.json: types, defaults, normalization, structural validation, atomic save,
│                   migrations.go; PeekLanguage for the startup dialog
├── state/          (m2) state.json: routine pause, last app version, per schedule last slot/result/error,
│                   migrations.go, Quarantine of a corrupt file; (m6) problems, partial, notified_signature/_at
├── migrate/        (m2) shared migration framework: append-only steps on the raw JSON, schema_version
├── fsutil/         (m2) atomic write (temp + fsync + rename), file/tree copy
├── backup/         (m2) Needed (when), Create (copy to backups/), Prune (keep the 10 most recent); (m8) Latest
├── startup/        (m2) Open: load → backup → quarantine → save migrated files → last_app_version; FatalText
├── devenv/         (m2) reads the dev-only env overrides (devenv.invalid on a bad value)
├── logging/        (m2) slog: JSON lines in logs/wchron.log (lumberjack: 5 MB × 5 files) + stderr text in
│                   dev builds; Read (newest first across rotated files, level filter); MaskPhone
├── i18n/           (m1) T(lang, key, params) over the locales catalog
├── logo/           logo geometry (package doc) → Draw(size) (SDF, anti-aliased), SDF helpers shared with
│   │               the tray, EncodeICO (PNG-compressed entries), SVG (vector copy); tests check that the
│   │               committed assets match
│   └── genicons/   `make icons`: writes build/windows/icon.ico (16–256 px), build/appicon.png,
│                   frontend/public/logo.svg
├── tokens/         (m3) date token engine (paths, captions, sheet names): Parse → Template.Render(t, lang),
│                   positioned token.* errors; syntax in the package doc and README.md
├── scheduler/      (m4) cadence.go: Validate (cadence.*, schedule.no_days) → plan; slots.go: per-day events
│                   (fire / DST gap / DST repeat), cursor, Next(cadence, from, n, loc); scheduler.go: the loop
│                   (Reload, Start, Stop, Wake; jitter, 5 min tolerance, aggregated slot.missed / slot.paused,
│                   slot.overlap, parallel schedules, NextRun → OnNext; (m6) RunNow: manual run, schedule.busy);
│                   runner.go: Runner, Job (Manual), Result (Status, Err, Problems), SlotText. Design in doc.go
├── resolver/       (m6) path pattern → concrete file(s) at the slot time: tokens, absolute, `*`/`?` in the name
│                   only (case-insensitive), match error | newest | all; Open (shared read, file.locked /
│                   file.unreadable), Validate for the editor (filespec.*). Design in the package doc;
│                   (m7) sendname.go: SendName / ValidateSendName (token template, extension appended,
│                   Windows name rules, filespec.send_name_invalid); (m8) SendNameNumbered: " (n)" when a
│                   custom name meets several matches of one spec
├── processor/      (m6) Processor interface (Type, OptionsSchema, Validate, Process → Output + cleanup), Env,
│   │               Registry (processor.unknown), Schemas; design in the package doc; (m7) Extensions (empty:
│   │               any file), Field.Tokens, Schema.extensions; (m8) OutputName (send name preview)
│   ├── passthrough/ (m6) "Original": temp copy of the original (Copy, also for other processors); no options
│   └── xlsxsnapshot/ (m7) "Values only" (type xlsx-snapshot, excelize): formulas → cached values (never
│                   recalculated), chosen sheets (`sheets`, tokens, empty = all), macros removed, always .xlsx;
│                   snapshot.go (design in the package doc, Process, ReadSheets), workbook.go (package
│                   structure and raw formula cells), clean.go (VBA parts, dangling rels/content types),
│                   names.go (defined names pointing at removed sheets)
├── provider/       (m5) provider.go: Provider, Channel, Pairer, LogoutChannel interfaces, Status/State,
│   │               Capabilities, Field schemas, Message; registry.go: Registry, Info; manager.go: Manager (open
│   │               enabled channels, connect, backoff reconnect 5 s→5 min, pairing, logout, rename, create,
│   │               delete + recipient cleanup, Usage, ValidateTarget, ListTargets, pending recipient checks on
│   │               connect, OnOutdated hook), ConfigStore, ErrTargetInvalid. Design in the package doc
│   │               (m6) send.go: Manager.Send (one send at a time per channel, 2–5 s pause between two,
│   │               cancellable), SendInfo, VerifyRecipient (check right before a send, stored in config)
│   ├── whatsapp/   (m5) whatsmeow linked device (doc.go: design): provider.go (capabilities, schemas, Open),
│   │   │           channel.go (status from events, pairing, logout, targets), send.go (documents, 100 MB,
│   │   │           one retry), target.go (E.164 / group JID), client.go (waClient interface), real.go
│   │   │           (whatsmeow adapter), session.go (session.db, corrupt → set aside), log.go (slog, masked)
│   │   └── smoke/  (m5) `make smoke`: read-only live check of the paired dev channels
│   └── fake/       (m5) in-memory provider: scriptable status, pairing, failures; records sends; (m6) OnSend hook
├── pipeline/       (m6) one run (scheduler.Runner): resolve + process every file once, slot.late before the first
│                   send, each recipient all files in order (channels in parallel), captions, capability and
│                   recipient checks, result ok | partial | failed | skipped, report to notify, temp cleanup,
│                   CleanTemp at startup. Design in the package doc; (m7) send names, xlsx warnings;
│                   (m8) numbered send names, "run finished" log line with name and error (run history)
├── notify/         (m6) report fan-out: toast + text to each error recipient; Signature, 6 h repeat, "works
│                   again" notice, notify.failed; manual runs toast only. Design in the package doc
├── toast/          (m6) notify.Toaster over the Wails notifications service (started by hand, never fatal;
│                   click → window on #/status)
├── update/         GitHub release check / download, driven by update.config.json
├── version/        (m1) Version constant (set by set-version.mjs); (m2) Dev (false with the `production` build tag)
├── tray/           (m1) tray menu, toggles, dynamic icon drawn at run time (Icon(state, keepAwake)): the
│                   logo + status dot (active green, paused grey, sending cyan, error red, disconnected
│                   purple) + yellow keep-awake dot; (m4) status lines: channels, next run; (m6) sending and
│                   error states from the Snapshot (sending > error > paused > disconnected > active)
└── platform/       interfaces + _windows.go / _other.go: SleepInhibitor, Autostart (m1); Paths (data dir
                    layout, Ensure → storage.unavailable), FatalDialog (MessageBoxW) (m2); (m6) files.go:
                    OpenShared (read, sharing read/write/delete), IsLocked, OpenExclusive (tests); (m8)
                    OpenFolder (Explorer); later
                    Protect/Unprotect (DPAPI, for channel secrets: with the first provider that has one).
                    Single instance is Wails' SingleInstance option (in main.go).
```

### Bindings (`internal/app`)

Today (m1, m2), `app.Service`: `State()`, `SetRoutineActive(on)`, `SetKeepAwake(on)` (live switch, not
persisted), `SetKeepAwakeDefault(on)` (setting: keep-awake at every start), `SetAutostart(on)` (registry
only), `SetTheme(theme)`, `SetLanguage(lang)`; each returns the new `Snapshot` `{routine_active,
keep_awake, keep_awake_default, autostart, theme, language, channels, version}` (`channels`: `none` |
`connected` | `disconnected`, the worst state among the configured channels). Theme, language and the
keep-awake default are saved to `config.json` (on failure `config.write_failed` and the change is rolled
back); the routine pause to `state.json` (a failure only logs `state.write_failed`).
`PreviewTokens(template)` (m3) renders a token template for now (`clock.Real`, injected in main) in the
current language; a malformed one returns its `token.*` error with `pos` (1-based character).
`PreviewRuns(cadence, n)` (m4) returns the next n run times from now (RFC 3339 with the local offset; n 0 = 5,
max 50) or the cadence's `cadence.*` / `schedule.no_days` error. The Snapshot also carries (m4)
`next_run` `{schedule_id, schedule_name, at}` | null: the earliest upcoming run (set by the scheduler, shown
in the tray, present also while paused).
Channels (m5): `Providers()` (`[{type, capabilities {files, max_file_size, max_caption_len, targets,
pairing}, settings_schema, target_schema}]`; fields `{key, kind: text|phone|select|secret|pick, required,
options, when}`, labels `provider.<type>.<schema>.<key>`), `ListChannels()` (`ChannelInfo` `{id, provider,
name, enabled, status {state, error {code, params} | null, account}, pairing}`; `state`: `connected` |
`connecting` | `disconnected` | `logged_out` | `not_paired` | `outdated` | `error`; `account` masked
"***567"), `CreateChannel(provider, name)`, `RenameChannel(id, name)`, `ChannelUsage(id)` (`{schedules:
[{id, name, recipients, disabled}], error_recipients}` for the delete dialog), `DeleteChannel(id)`
(`{was_paired, logged_out}`), `StartPairing(id)` (a paired channel is logged out first), `CancelPairing(id)`,
`Logout(id)`, `ValidateTarget(channelID, target)` (`{target (normalized), display_name, verified, warning}`:
offline it is `verified: "unknown"` with the `recipient.check_unavailable` warning), `ListTargets(channelID)`
(`[{target, display_name}]`). The Snapshot (m5) also carries `channel_list` (`ChannelInfo[]`) and the real
`channels` summary (worst state among the enabled channels).
Runs (m6): `SendNow(scheduleID)` starts a manual run at once (any schedule, enabled or not; no days, window or
tolerance; `schedule.busy` while that schedule runs, `schedule.not_found`; acts for real; problems → toast
only) and returns when it has started; `AckErrors()` clears the tray's error state (called by the Status page
on mount) and returns the Snapshot. The Snapshot (m6) also carries `sending` (any run in progress) and
`run_error` (a schedule failed or sent only in part and hasn't succeeded since, not acknowledged).
`Logs(level, limit)` (m2) returns the most recent log entries `{time, level, msg, attrs}`, newest first
(`level`: minimum, `""` for all; `limit` 0 = 200, max 5000; `logs.read_failed`); its viewer is milestone 8.
`frontend/bindings` is generated (`make bindings`, and by every build) and git-ignored.

Excel (m7): `ReadSheets(pathPattern)` resolves the pattern now (tokens for now, current language; a wildcard
takes the newest match) and returns the workbook's sheet names in workbook order, without macro sheets
(`filespec.*`, `token.*`, `file.*`, `xlsx.not_xlsx`, `xlsx.open_failed`). The reader is set from main.go
with `app.SetSheetReader` (a plain function: exported `Service` methods become bindings).
Schedules (m8): `ListSchedules()` (`ScheduleSummary` `{schedule, last_result, last_error, last_run_at,
next_run, invalid}`: next run from now, null when disabled or invalid), `GetSchedule(id)`, `NewSchedule()`
(defaults, empty id), `NewFileSpec()`, `NewRecipientID()`, `SaveSchedule(schedule)` → `SaveResult`
`{schedule | null, problems: [{field, code, params}]}` (validate.go: cadence via `scheduler.Validate`,
recipients (`recipient.required`, `recipient.channel_missing`, `recipient.duplicate`), caption and per-file
captions (`token.*`), files (`schedule.no_files`, `resolver.Render`, `processor.*` incl.
`processor.extension_unsupported`, `resolver.ValidateSendName`), `schedule.name_required`; fields like
`cadence.window`, `recipients.<id>`, `files.<id>.path_pattern`; empty id = new, appended), `DuplicateSchedule(id)`
(after the original, disabled, `schedule.copy_name`), `DeleteSchedule(id)` (permanent, also its state.json
entry), `SetScheduleEnabled(id, on)` (turning on validates: first problem as the error). Every change: atomic
save through `Store.UpdateConfig` (`config.write_failed`), then the `Changed` hook. Recipients are checked by the
GUI's picker with `ValidateTarget` when added; the save trusts their `verified`.
Previews (m8): `ProcessorSchemas()` (`processor.Registry.Schemas`), `PreviewFiles(pattern, match)`
(`FilePreview` `{path, files: [{name, size, mod_time}], error}`: problems as data), `PreviewSendName(template,
pattern, match, processor)` (the names a run would send now, `resolver.SendNameNumbered` + `OutputName`).
Settings (m8): `ErrorRecipients()`, `SaveErrorRecipients(list)` (problems under `error_recipients.<id>`),
`DataDir()`, `OpenDataDir()`, `OpenLogsDir()` (`folder.open_failed`), `LastBackup()` (RFC 3339 or null).
Status (m8): `RunHistory(limit)` (0 = 50, max 500) → `RunRecord` `{time, schedule_id, schedule_name, manual,
status, sent, planned, error}` read from the log lines "pipeline: run finished", "pipeline: run skipped, too
late…", "scheduler: slot skipped", "scheduler: slots skipped while paused" (history.go: keep their attributes).
Wiring setters are plain functions: `app.SetSheetReader`, `app.Configure(svc, Extras{…})`.
Planned: `CheckUpdate`.
Errors come back as `apperr.Error` serialized as `{code, params}`.

Events (backend → frontend): `app:state` (m1: full Snapshot after every change, from the window or the
tray; replaces the planned `routine:toggled`; m4: also when the next run changes, so there is no
`schedule:next` event), (m5) `channel:status` (the `ChannelInfo` that changed) and `channel:qr` (pairing steps
`{channel_id, kind: code|success|timeout|failed|cancelled, code, error}`),
(m6) `run:started` and `run:finished` (`RunEvent` `{schedule_id, schedule_name, manual, status, problems}`;
`status` "" when started), (m8) `schedules:changed` (`{id}`, "" when several: the list reloads),
`update:available`.

### Data and state

All under the data dir (default `%APPDATA%\wchron`, override `WCHRON_DATA_DIR`; created at startup by
`platform.Paths.Ensure`, `storage.unavailable` if it can't be written) (m2):

| Path | Content |
| --- | --- |
| `config.json` | `schema_version`, `settings`, `channels[]`, `schedules[]` (fields below); written on the first change, not at the first start |
| `state.json` | `schema_version`, `routine_active`, `last_app_version`, `notified_update_version`, per schedule: last fired slot, last result, last error, problems, notification memory |
| `state.json.corrupt-<YYYYMMDD-HHMMSS>` | an unreadable `state.json` moved aside at startup (`state.read_failed`); never deleted by the app |
| `channels/<id>/` | per-channel data (m5): WhatsApp `session.db` (whatsmeow sqlstore, `modernc.org/sqlite`; never logged); an unreadable one is renamed `session.db.corrupt-<YYYYMMDD-HHMMSS>` (never deleted by the app). Later: DPAPI-encrypted secrets of other providers. Created with the channel, deleted with it |
| `logs/` | `wchron.log` (JSON lines) + up to 4 rotated `wchron-<timestamp>.log` (about 5 MB each, not compressed) |
| `backups/<YYYYMMDD-HHMMSS>-v<old version>/` | copies of `config.json`, `state.json` and `channels/` (with the SQLite `-wal`/`-shm` files) made at startup before a migration, when the app version changed since the last run, or when `state.json` is missing next to a `config.json`. The 10 most recent are kept, older ones deleted (app-generated copies, not user data) |

Temp copies for processors go in `%TEMP%\wchron\<run-id>\<file>-<match>\` (m6) and are removed at the end of
every run, also on failure; leftovers of a crash are removed at startup (`pipeline.CleanTemp`).

**Formats and startup** (m2). Both files carry `schema_version` (current: 1) and change only through
append-only steps in `internal/config/migrations.go` / `internal/state/migrations.go` (framework:
`internal/migrate`). Writes are atomic (temp file + fsync + rename). At startup, `startup.Open` loads both
in memory (migrating in memory), backs up when needed (`storage.backup_failed` stops the start), moves a
corrupt `state.json` aside, saves a migrated `config.json`, then records `last_app_version`. A file
newer than the app (`config.newer_version`), an unreadable `config.json` (`config.read_failed`), a
structurally invalid one (`config.invalid`: empty or duplicate ids, unknown enums) or a failed migration
(`config.migration_failed`) stops the start with a native error dialog, leaving the files as they are.
Missing fields are filled on load (nil lists become `[]`, empty enums their default, missing raw objects `{}`).

**`config.json`**

- `schema_version`
- `settings`: `error_recipients[]` (Recipient, 0 or more), `keep_awake_default` (false), `language`
  (`it` default | `en`), `theme` (`system` default | `light` | `dark`).
  No time zone setting: always the system one. Autostart is not stored: the HKCU Run key is the truth.
- `channels[]`: Channel
- `schedules[]`: Schedule

**Channel** (one configured account of a provider; several per provider allowed)

- `id`, `provider` (`"whatsapp"`, later `"telegram"`, `"ntfy"`…), `name` (user label, e.g. "WhatsApp
  personale"), `enabled`
- `settings`: provider-specific object, non-secret (secrets and sessions live in `channels/<id>/`)

**Recipient**

- `id` (unique within its list), `channel_id`, `target`: provider-specific (WhatsApp: `{kind: "number", number}`
  E.164 or `{kind: "group", jid}`), `display_name` (last known name, for display only)
- `verified` (m5): `"unknown"` (default: saved while the channel was not connected, checked automatically at
  its next connection), `"yes"` or `"no"` (rejected by the service: never sent to, `recipient.check_failed`
  in the log). Added to v1 in place (v1 was never released), no migration step.

**Schedule**

- `id`, `name`, `enabled`
- `cadence`: `{ mode: "interval" | "fixed", interval: {hours, minutes}, anchor: "HH:MM",
  window: {start, end} | null, times: ["HH:MM"], days: ["mon"…"sun"] }`
- `recipients[]`: Recipient, at least one, on one or more channels
- `caption`: optional token template (`""` for none)
- `files[]`: ordered FileSpec

**FileSpec**

- `id`
- `path_pattern`: absolute path with tokens and/or glob in the file name
- `match`: `"error"` (default) | `"newest"` | `"all"`
- `processor`: `"passthrough"` (default; "Original" in the GUI) | `"xlsx-snapshot"` ("Values only") | future types
- `processor_options`: processor-specific (xlsx-snapshot: `{ "sheets": ["{date:DD-MM-YYYY}", "Summary"] }`,
  empty or absent list = all sheets)
- `caption`: optional override of the schedule caption (absent: inherit; present, even `""`: override)
- `send_name` (m7): optional token template, the file name the recipients see (absent or `""`: the processed
  file's name; the output's extension is appended when the name has none). Added to v1 in place (v1 not
  released), no migration step

**`state.json`**: `schema_version`, `routine_active` (default true: the tray pause survives a restart),
`last_app_version` (triggers the backup on upgrade), `notified_update_version`, `schedules`: map by
schedule id of `{last_slot, last_result, last_error, last_run_at}` (m4: `last_slot` is the instant of the
last slot that started a run, RFC 3339 with its offset, so the two autumn occurrences differ and a slot never
runs twice; `last_run_at` when it started; `last_result` `ok` | `dry_run` (stub runner before 0.6.0, nothing sent) |
`skipped` | `failed` | (m6) `partial`; `last_error` `{code, params}` of the most severe problem or the skip reason, or
null; skips leave `last_slot`/`last_run_at` alone; times omitted when unset). (m6, added to v1 in place: v1 not
released) `problems` (every problem of the last run, per file/recipient; a skip lists its reason),
`notified_signature` / `notified_at` (notify's memory of the last reported problem set, cleared by the "works
again" notice). A manual run sets `last_run_at` only.

### Configuration (dev-only env overrides)

| Env var | Default | Meaning |
| --- | --- | --- |
| `WCHRON_DATA_DIR` | `%APPDATA%\wchron` | Data dir root (config, state, session, logs, backups) |
| `WCHRON_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `WCHRON_UPDATE_DISABLED` | `false` | Skip the update check (dev builds; parsed since m2, used from m10) |

Read by `internal/devenv` (m2); an invalid value stops the start (`devenv.invalid`). A relative
`WCHRON_DATA_DIR` is resolved against the working directory. Dev builds (no `production` tag) also log
to stderr.

### Updates

`update.config.json` is read at build time (embedded with `go:embed`), not at run time: it says where
releases come from, how to pick the installer and its checksum, when to check (startup, channel
failures such as WhatsApp rejecting the client, at most once per hour) and how far the next slot must be before installing. Flow: notify only;
on the user's click, download + checksum, wait for a quiet moment, quit and run the installer silently
(user view in README.md "Updates"). `internal/update` does the GitHub check and download; `internal/app` owns the
install timing (it knows the scheduler).

## Frontend (`frontend/src`)

Layout (chosen from the mockups, "Sidebar chiara"): fixed 224 px left sidebar (Schedules, Status and
log, Channels, Settings) with the channels state and the routine toggle at its bottom; pages are a
22 px title plus white cards/tables on a light grey ground. Themes: light, dark and system (follows
Windows), all from the semantic tokens in `app.css`, switched by `data-theme` on `<html>`. Svelte 5
runes, TypeScript strict, Tailwind 4 (`@tailwindcss/vite`), Inter from `@fontsource-variable/inter`.
Routing is a minimal hash router (`#/schedules`, `#/schedules/<id>` or `/new` for the editor, `#/settings`…).
(m1) marks what exists today.

| File | Responsibility |
| --- | --- |
| `App.svelte` | (m1) shell: sidebar + current page; starts the router and the state sync |
| `components/Sidebar.svelte` | (m1) logo, nav, channels state (m5: the real summary, links to Channels) + "Activate routine" toggle |
| `components/Logo.svelte` | the logo as inline SVG (`size` prop, aria-hidden); brand colours are raw values here |
| `components/Toggle.svelte` | (m1) switch: `<button aria-pressed>` |
| `components/Icon.svelte` | (m1) inline stroke icons of the nav; (m8) copy, trash, up, down, close, folder |
| `components/EmptyPage.svelte` | (m1) placeholder page: title + empty-state card |
| `pages/Schedules.svelte` | (m8) list (enable toggle saving at once, when/recipients summary, next run, last result with the first problem; duplicate, delete with confirmation; empty states: no channel → Channels, no schedule → new); under `#/schedules/<id>` it renders the editor |
| `pages/ScheduleEditor.svelte` | (m8) the editor: draft until Save (`saveSchedule`, problems under their fields), unsaved-changes guard (router), name, cadence, recipients, caption preview, files (reorder with arrows), next 5 runs, active, send now (saved only, confirmation), delete |
| `components/editor/CadenceEditor.svelte` | (m8) interval / fixed times, anchor, time range (overnight hint), days with Weekdays / All |
| `components/editor/FileSpecCard.svelte` | (m8) one file spec: path + `previewFiles`, match mode, processor (filtered by extension) and its schema options (list → chips with token previews; `sheets` → read from the file), own caption, send name + `previewSendName` |
| `components/Field.svelte`, `SegmentedControl.svelte`, `ChipsInput.svelte`, `Badge.svelte`, `TimeInput.svelte` | (m8) shared form pieces |
| `components/RecipientList.svelte`, `RecipientPicker.svelte` | (m8) recipients with channel, kind and verified badge; add = channel choice + the provider's target picker |
| `pages/Status.svelte` | (m8) cards (channels, today, problems in 7 days, keep awake), last 50 runs (`runHistory`), log viewer with level filter and open folder; m6: calls `ackErrors` on mount |
| `pages/Settings.svelte` | (m8) error recipients (saved at once), data folder + last backup + open; m1: theme, language, autostart, keep awake, version; m2: keep awake at startup; later: updates |
| `pages/Channels.svelte` | (m5) channels list with live status (from the Snapshot), add channel (provider + name → its setup), pair, re-pair, log out, rename, delete (confirmations naming the channel; delete lists the affected schedules) |
| `providers/index.ts` | (m5) per-provider UI by type (label, setup component; (m8) target picker, target kind): the only frontend place naming a provider |
| `providers/whatsapp/Setup.svelte` | (m5) ToS note, steps on the phone, live QR, outcome |
| `providers/whatsapp/TargetPicker.svelte`, `target.ts` | (m8) Number (ValidateTarget; channel not open → unverified) / Group (search over ListTargets, needs the connection); the target's kind label |
| `components/QrCode.svelte` | (m5) QR as SVG, `uqr` loaded lazily; dark on light in every theme |
| `components/ConfirmDialog.svelte` | (m5) modal `<dialog>` confirmation (title, consequences, danger button) |
| `lib/pairing.svelte.ts` | (m5) pairing steps per channel from `channel:qr` |
| `lib/api.ts` | (m1) the only module calling the bindings: `AppError` `{code, params}` mapping, null normalization, `onState`; (m2) `logs`; (m3) `previewTokens`; (m4) `previewRuns`; (m5) channel calls, `fromRef`, `onPairing`; (m6) `sendNow`, `ackErrors`, `onRunFinished`; (m7) `readSheets`; (m8) schedule calls (`normSchedule`), previews, settings, `runHistory`, `onSchedulesChanged`, `Problem` |
| `lib/state.svelte.ts` | (m1) Snapshot mirrored from Go (calls + `app:state` events), applies theme and language |
| `lib/router.svelte.ts` | (m1) hash router; (m8) `route.param`, leave guard (`setLeaveGuard`, `leave.pending`, `proceed`, `stay`), `go` |
| `lib/format.ts` | (m8) dates relative to today, sizes, cadence and days summaries |
| `lib/i18n.svelte.ts` | (m1) `t()` / `tError()` over `../locales/*.json` (it typed as `typeof en`), current language |
| `app.css` | (m1) Tailwind 4 tokens (light + dark) and `@utility` classes |
| `../playwright.config.ts`, `../e2e/app.spec.ts` | (m8) `make e2e`: Vite in `--mode e2e` (port 9246) + Chromium (`PW_CHANNEL=msedge` for the system browser), clock fixed at Mon 26 Oct 2026 09:00 Europe/Rome; `E2E_SHOTS=<dir>` saves screenshots |
| `../e2e/mock/` | (m8) fake backend: `service.ts` (every `app.Service` method, in-memory, nothing sent; a test checks the list), `logic.ts` (tokens, next runs, cadence checks, send names), `runtime.ts` (events), `options.ts` (`window.__MOCK_OPTIONS`, `window.__mock.calls`); swapped in by `vite.config.ts` |
| `../public/logo.svg` | favicon (`index.html`), copied to `dist/` by Vite; generated by `make icons` |

## Commands (`make help`)

| Target | What it does |
| --- | --- |
| `make deps` | `npm install` in `frontend/` |
| `make dev` | `wails3 dev` with hot reload |
| `make build` | `bin/wchron.exe` (bindings + frontend + syso + go build, via `Taskfile.yml`) |
| `make package` | `make build` + `bin/wchron-amd64-installer.exe` (NSIS, per user; `MAKENSIS` defaults to `C:/Program Files (x86)/NSIS/makensis.exe`) |
| `make run` | build and start the exe |
| `make bindings` | regenerate `frontend/bindings` |
| `make icons` | regenerate the logo assets from `internal/logo` (`icon.ico`, `appicon.png`, `logo.svg`; committed) |
| `make test` / `make vet` | `go test ./...` / `go vet ./...` + Linux vet of the packages without Wails |
| `make smoke` | opt-in, read-only live check: connects the paired WhatsApp channels of `WCHRON_DATA_DIR` (required), prints status and group count, disconnects; never sends. Stop `make dev` first |
| `make check` | bindings, vet, test, `svelte-check --fail-on-warnings`, frontend build: must be 0 errors, 0 warnings |
| `make e2e` | bindings + Playwright UI tests against the fake backend (no Wails, no disk); required before a release |
| `make version V=x.y.z` | `scripts/set-version.mjs` |
| `make clean` | remove `bin/`, `frontend/dist`, `.task/` |
