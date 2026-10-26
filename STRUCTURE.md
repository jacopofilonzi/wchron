# Project structure

wchron is a Windows tray app for one user: at scheduled times it resolves file paths (with date tokens),
optionally reduces Excel workbooks to selected sheets, and sends the files to one or more recipients on
one or more channels. Delivery goes through provider plug-ins: WhatsApp (whatsmeow linked device) first,
Telegram/ntfy possible later without structural changes. One Go process holds the core (scheduler,
pipeline, channels, state); the Svelte GUI in a Wails window talks to it through generated bindings and events.

> Status: milestones 1 (skeleton) and 2 (data, logging) are in place; the rest of this file is the planned
> layout. Packages and files that exist today are marked **(m1)** / **(m2)**. Update this file as each
> milestone lands.

## Milestones

- [x] 1. Skeleton: Wails 3 + Svelte + Tailwind 4, tray with the two toggles and dynamic icon, single
      instance, autostart, Makefile, i18n catalog, version script.
- [x] 2. Data dir, config, state, migrations + backup, logging (rotating files, reader, masking), platform
      paths and fatal dialog, error registry.
- [ ] 3. Token engine with tests (Italian names, offsets, escaping).
- [ ] 4. Scheduler with tests (interval, fixed times, window, days, jitter, skipped slots, DST both ways, fake clock).
- [ ] 5. Provider layer (interface, registry, channels, fake provider, DPAPI secrets) + WhatsApp provider:
      QR pairing, session persistence, number check, group list, document send.
- [ ] 6. Pipeline with `passthrough`, resolver, multiple files × multiple recipients, pauses, error
      aggregation, notifications to the error recipients.
- [ ] 7. `xlsx-sheets` processor with tests on synthetic workbooks (formulas, missing sheet, locked file,
      no cached values).
- [ ] 8. GUI screens (channels page, recipient picker), previews, delete confirmations, error recipients.
- [ ] 9. Hardening: reconnection, installer, release workflow (log rotation landed with milestone 2).
- [ ] 10. Update notification and self-update.

```text
wchron/
├── main.go             wiring (m1, m2): Wails app + single instance first, then devenv → paths → log file →
│                       startup.Open (config/state load, backup, migrate) → store → service, window, tray
├── internal/           Go (go.mod: 1.26, toolchain 1.27) — core and platform layer (see below)
├── locales/            it.json, en.json (shared i18n catalog, flat namespaced keys) + embed.go (m1)
├── frontend/           Svelte 5 + TS + Vite + Tailwind 4 — GUI
├── build/              Wails build assets (m1): config.yml, Taskfile.yml (frontend, bindings),
│                       windows/ (Taskfile.yml, icon.ico, wails.exe.manifest, info.json, nsis/ per-user installer)
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
│                   subscribers) + Service (the Wails bindings) + events
├── apperr/         (m1) Error{Code, Params, Err}, codes.go (registry mirrored by ERRORS.md), JSON `{code, params}`
├── clock/          Clock interface, real and fake
├── config/         (m2) config.json: types, defaults, normalization, structural validation, atomic save,
│                   migrations.go; PeekLanguage for the startup dialog
├── state/          (m2) state.json: routine pause, last app version, per schedule last slot/result/error,
│                   migrations.go, Quarantine of a corrupt file
├── migrate/        (m2) shared migration framework: append-only steps on the raw JSON, schema_version
├── fsutil/         (m2) atomic write (temp + fsync + rename), file/tree copy
├── backup/         (m2) Needed (when), Create (copy to backups/), Prune (keep the 10 most recent)
├── startup/        (m2) Open: load → backup → quarantine → save migrated files → last_app_version; FatalText
├── devenv/         (m2) reads the dev-only env overrides (devenv.invalid on a bad value)
├── logging/        (m2) slog: JSON lines in logs/wchron.log (lumberjack: 5 MB × 5 files) + stderr text in
│                   dev builds; Read (newest first across rotated files, level filter); MaskPhone
├── i18n/           (m1) T(lang, key, params) over the locales catalog
├── tokens/         date token engine (paths, captions, sheet names), validation, preview
├── scheduler/      cadence → slots (interval, fixed times, window, days), jitter, DST, tolerance, next N runs
├── resolver/       path pattern → concrete file(s); glob match mode: error | newest | all
├── processor/      Processor interface + registry; passthrough/, xlsxsheets/
├── provider/       Provider + Channel interfaces, registry, capabilities, settings/target schemas; channel
│   │               manager (open/close/status of every configured channel)
│   ├── whatsapp/   whatsmeow: QR pairing, logout, IsOnWhatsApp, groups, upload+send document
│   └── fake/       in-memory provider for tests
├── pipeline/       one run: per file resolve → process once → send to each recipient; 2–5 s pauses; error report
├── notify/         report fan-out: desktop toast + text to each error recipient (any channel)
├── update/         GitHub release check / download, driven by update.config.json
├── version/        (m1) Version constant (set by set-version.mjs); (m2) Dev (false with the `production` build tag)
├── tray/           (m1) tray menu, toggles, dynamic icon drawn at run time (Icon(state, keepAwake))
└── platform/       interfaces + _windows.go / _other.go: SleepInhibitor, Autostart (m1); Paths (data dir
                    layout, Ensure → storage.unavailable), FatalDialog (MessageBoxW) (m2); later Toast,
                    Protect/Unprotect (DPAPI, for channel secrets).
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
`Logs(level, limit)` (m2) returns the most recent log entries `{time, level, msg, attrs}`, newest first
(`level`: minimum, `""` for all; `limit` 0 = 200, max 5000; `logs.read_failed`); its viewer is milestone 8.
`frontend/bindings` is generated (`make bindings`, and by every build) and git-ignored.

Planned surface (names final at implementation): schedules CRUD (`ListSchedules`, `SaveSchedule`,
`DeleteSchedule`), `PreviewRuns(cadence, n)`, `PreviewTokens(template)`,
`ReadSheets(path)`, `SendNow(id)`, settings get/save, channels (`Providers` with capabilities and schemas,
`ListChannels`, `SaveChannel`, `DeleteChannel` (returns the affected schedules first via `ChannelUsage`),
`ChannelStatus`, `StartPairing(id)`,
`Logout(id)`, `ValidateTarget(channel, target)`, `ListTargets(channel)`), `CheckUpdate`,
`ProcessorSchemas`.
Errors come back as `apperr.Error` serialized as `{code, params}`.

Events (backend → frontend): `app:state` (m1: full Snapshot after every change, from the window or the
tray; replaces the planned `routine:toggled`), `channel:status` and `channel:qr` (with the channel id),
`run:started`, `run:finished`,
`update:available`.

### Data and state

All under the data dir (default `%APPDATA%\wchron`, override `WCHRON_DATA_DIR`; created at startup by
`platform.Paths.Ensure`, `storage.unavailable` if it can't be written) (m2):

| Path | Content |
| --- | --- |
| `config.json` | `schema_version`, `settings`, `channels[]`, `schedules[]` (fields below); written on the first change, not at the first start |
| `state.json` | `schema_version`, `routine_active`, `last_app_version`, `notified_update_version`, per schedule: last fired slot, last result, last error |
| `state.json.corrupt-<YYYYMMDD-HHMMSS>` | an unreadable `state.json` moved aside at startup (`state.read_failed`); never deleted by the app |
| `channels/<id>/` | per-channel data: WhatsApp `session.db` (whatsmeow, `modernc.org/sqlite`), DPAPI-encrypted secrets of other providers |
| `logs/` | `wchron.log` (JSON lines) + up to 4 rotated `wchron-<timestamp>.log` (about 5 MB each, not compressed) |
| `backups/<YYYYMMDD-HHMMSS>-v<old version>/` | copies of `config.json`, `state.json` and `channels/` (with the SQLite `-wal`/`-shm` files) made at startup before a migration, when the app version changed since the last run, or when `state.json` is missing next to a `config.json`. The 10 most recent are kept, older ones deleted (app-generated copies, not user data) |

Temp copies for processors go in `%TEMP%\wchron\` and are removed after each send.

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

- `id`, `channel_id`, `target`: provider-specific (WhatsApp: `{kind: "number", number}` E.164 or
  `{kind: "group", jid}`), `display_name` (last known name, for display only)

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
- `processor`: `"passthrough"` (default) | `"xlsx-sheets"` | future types
- `processor_options`: processor-specific (e.g. `{ "sheets": ["{date:DD-MM-YYYY}", "Summary"] }`)
- `caption`: optional override of the schedule caption (absent: inherit; present, even `""`: override)

**`state.json`**: `schema_version`, `routine_active` (default true: the tray pause survives a restart),
`last_app_version` (triggers the backup on upgrade), `notified_update_version`, `schedules`: map by
schedule id of `{last_slot, last_result, last_error, last_run_at}` (`last_slot` drops the repeated autumn DST
slot; times RFC 3339, omitted when unset; `last_error` is `{code, params}` or null; `last_result` values come
with the pipeline).

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
Routing is a minimal hash router (`#/schedules`, `#/settings`…). (m1) marks what exists today.

| File | Responsibility |
| --- | --- |
| `App.svelte` | (m1) shell: sidebar + current page; starts the router and the state sync |
| `components/Sidebar.svelte` | (m1) logo, nav, channels state + "Activate routine" toggle |
| `components/Toggle.svelte` | (m1) switch: `<button aria-pressed>` |
| `components/Icon.svelte` | (m1) inline stroke icons of the nav |
| `components/EmptyPage.svelte` | (m1) placeholder page: title + empty-state card |
| `pages/Schedules.svelte` | list + editor (cadence, recipients on any channel, caption, files, next 5 runs, send now); m1: placeholder |
| `pages/Status.svelte` | connection, last runs, errors, log viewer; m1: placeholder |
| `pages/Settings.svelte` | error recipients, autostart, keep awake, language, theme, updates; m1: theme, language, autostart, keep awake, version; m2: keep awake at startup |
| `pages/Channels.svelte` | channels list with status, add channel (pick provider → its setup component), rename, delete (with confirmation); m1: placeholder |
| `providers/<type>/` | per-provider UI: setup (WhatsApp: QR pairing, re-pair, logout) and target picker |
| `lib/api.ts` | (m1) the only module calling the bindings: `AppError` `{code, params}` mapping, null normalization, `onState`; (m2) `logs` |
| `lib/state.svelte.ts` | (m1) Snapshot mirrored from Go (calls + `app:state` events), applies theme and language |
| `lib/router.svelte.ts` | (m1) hash router |
| `lib/i18n.svelte.ts` | (m1) `t()` / `tError()` over `../locales/*.json` (it typed as `typeof en`), current language |
| `app.css` | (m1) Tailwind 4 tokens (light + dark) and `@utility` classes |
| `e2e/mock/` | fake backend for Playwright |

## Commands (`make help`)

| Target | What it does |
| --- | --- |
| `make deps` | `npm install` in `frontend/` |
| `make dev` | `wails3 dev` with hot reload |
| `make build` | `bin/wchron.exe` (bindings + frontend + syso + go build, via `Taskfile.yml`) |
| `make package` | `make build` + `bin/wchron-amd64-installer.exe` (NSIS, per user; `MAKENSIS` defaults to `C:/Program Files (x86)/NSIS/makensis.exe`) |
| `make run` | build and start the exe |
| `make bindings` | regenerate `frontend/bindings` |
| `make test` / `make vet` | `go test ./...` / `go vet ./...` + Linux vet of the packages without Wails |
| `make check` | bindings, vet, test, `svelte-check --fail-on-warnings`, frontend build: must be 0 errors, 0 warnings |
| `make version V=x.y.z` | `scripts/set-version.mjs` |
| `make clean` | remove `bin/`, `frontend/dist`, `.task/` |
