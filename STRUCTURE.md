# Project structure

wchron is a Windows tray app for one user: at scheduled times it resolves file paths (with date tokens),
optionally reduces Excel workbooks to selected sheets, and sends the files to a WhatsApp number or group
through a whatsmeow linked-device session. One Go process holds the core (scheduler, pipeline, WhatsApp
client, state); the Svelte GUI in a Wails window talks to it through generated bindings and events.

> Status: planned layout, nothing is implemented yet. Update this file as each milestone lands.

## Milestones

- [ ] 1. Skeleton: Wails 3 + Svelte + Tailwind 4, tray with the two toggles and dynamic icon, single
      instance, autostart, Makefile, i18n catalog, version script.
- [ ] 2. Data dir, config, state, migrations + backup, logging, platform interfaces (Windows), error registry.
- [ ] 3. Token engine with tests (Italian names, offsets, escaping).
- [ ] 4. Scheduler with tests (interval, fixed times, window, days, jitter, skipped slots, DST both ways, fake clock).
- [ ] 5. WhatsApp layer: QR pairing, session persistence, number check, group list, document send.
- [ ] 6. Pipeline with `passthrough`, resolver, multiple files, pauses, error aggregation, notifications.
- [ ] 7. `xlsx-sheets` processor with tests on synthetic workbooks (formulas, missing sheet, locked file,
      no cached values).
- [ ] 8. GUI screens, previews, trash, error number.
- [ ] 9. Hardening: reconnection, log rotation, installer, release workflow.
- [ ] 10. Update notification and self-update.

```text
wchron/
├── main.go             wiring: data dir, logging, config/state load + backup/migrate, services, window, tray
├── internal/           Go 1.25 — core and platform layer (see below)
├── locales/            it.json, en.json (shared i18n catalog) + embed.go
├── frontend/           Svelte 5 + TS + Vite + Tailwind 4 — GUI
├── build/              Wails build assets: config.yml, windows/ (icon, manifest, info.json, nsis/)
├── scripts/            set-version.mjs
├── .github/workflows/  release.yml (on a v* tag only: build + release)
├── update.config.json  build-time update settings, embedded in the binary
├── Taskfile.yml        Wails tasks (build, package, bindings)
├── .env.example        dev-only overrides; `.env` is git-ignored
├── Makefile            entry point for every command (`make help`)
├── README.md           user docs: behaviour, install, updates, ToS risk
├── ERRORS.md           every user-visible error code
└── bin/                build output: wchron.exe, wchron-amd64-installer.exe (git-ignored)
```

## Backend (`internal/`)

```text
internal/
├── app/            the Wails service exposed to the frontend (bindings) + events; thin, calls the core
├── apperr/         Error{Code, Params, Err}, codes.go (registry mirrored by ERRORS.md)
├── clock/          Clock interface, real and fake
├── config/         config.json: types, defaults, validation, atomic save, migrations.go, trash
├── state/          state.json: last fired slot/result/error per schedule, migrations.go
├── backup/         copies data files to backups/ before migrations and on version change
├── devenv/         reads the dev-only env overrides
├── logging/        slog setup, rotating files, in-app log reader, number masking
├── i18n/           T(lang, key, params) over the locales catalog
├── tokens/         date token engine (paths, captions, sheet names), validation, preview
├── scheduler/      cadence → slots (interval, fixed times, window, days), jitter, DST, tolerance, next N runs
├── resolver/       path pattern → concrete file(s); glob match mode: error | newest | all
├── processor/      Processor interface + registry; passthrough/, xlsxsheets/
├── sender/         Sender interface; WhatsApp implementation; Fake for tests
├── wa/             whatsmeow wrapper: connect, QR pairing, logout, IsOnWhatsApp, groups, upload+send document
├── pipeline/       one run: resolve → process → send per file, 2–5 s pauses, error aggregation
├── notify/         Notifier fan-out: desktop toast + optional WhatsApp error number
├── update/         GitHub release check / download, driven by update.config.json
├── version/        Version constant (set by set-version.mjs)
├── tray/           tray menu, toggles, dynamic icon states
└── platform/       interfaces + _windows.go: SleepInhibitor, Autostart, Paths, Toast, single instance, recycle bin
```

### Bindings (`internal/app`)

Planned surface (names final at implementation): schedules CRUD + trash (`ListSchedules`, `SaveSchedule`,
`TrashSchedule`, `RestoreSchedule`, `EmptyTrash`), `PreviewRuns(cadence, n)`, `PreviewTokens(template)`,
`ReadSheets(path)`, `SendNow(id)`, settings get/save, WhatsApp (`Status`, `StartPairing`, `Logout`,
`CheckNumber`, `ListGroups`), `Logs(filter)`, `CheckUpdate`, `ProcessorSchemas`.
Errors come back as `apperr.Error` serialized as `{code, params}`.

Events (backend → frontend): `wa:status`, `wa:qr`, `run:started`, `run:finished`, `routine:toggled`,
`update:available`.

### Data and state

All under the data dir (default `%APPDATA%\wchron`, override `WCHRON_DATA_DIR`):

| Path | Content |
| --- | --- |
| `config.json` | `schema_version`, `settings`, `schedules[]`, `trash[]` (fields below) |
| `state.json` | `schema_version`, per schedule: last fired slot, last result, last error |
| `session.db` | whatsmeow SQLite store (`modernc.org/sqlite`) |
| `logs/` | rotating `wchron.log` files |
| `backups/<timestamp>-v<version>/` | copies made before migrations and app upgrades |

Temp copies for processors go in `%TEMP%\wchron\` and are removed after each send.

**`config.json`**

- `schema_version`
- `settings`: `error_number` (E.164, optional), `autostart`, `keep_awake_default`, `language` (`it` | `en`),
  `theme` (`system` default | `light` | `dark`).
  No time zone setting: always the system one.
- `schedules[]`: Schedule
- `trash[]`: `{ deleted_at, schedule }`, restorable until "empty trash"

**Schedule**

- `id`, `name`, `enabled`
- `cadence`: `{ mode: "interval" | "fixed", interval: {hours, minutes}, anchor: "HH:MM",
  window: {start, end} | null, times: ["HH:MM"], days: ["mon"…"sun"] }`
- `recipient`: `{ type: "number" | "group", value: <E.164 number or group JID>, display_name }`
- `caption`: optional token template
- `files[]`: ordered FileSpec

**FileSpec**

- `id`
- `path_pattern`: absolute path with tokens and/or glob in the file name
- `match`: `"error"` (default) | `"newest"` | `"all"`
- `processor`: `"passthrough"` | `"xlsx-sheets"` | future types
- `processor_options`: processor-specific (e.g. `{ "sheets": ["{date:DD-MM-YYYY}", "Summary"] }`)
- `caption`: optional override of the schedule caption

**`state.json`**: `schema_version`, per schedule id: `last_slot` (used to drop the repeated autumn DST
slot), `last_result`, `last_error` (`{code, params}`), `last_run_at`. Plus `last_app_version` (triggers
the backup on upgrade) and `notified_update_version`.

### Configuration (dev-only env overrides)

| Env var | Default | Meaning |
| --- | --- | --- |
| `WCHRON_DATA_DIR` | `%APPDATA%\wchron` | Data dir root (config, state, session, logs, backups) |
| `WCHRON_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `WCHRON_UPDATE_DISABLED` | `false` | Skip the update check (dev builds) |

### Updates

`update.config.json` is read at build time (embedded with `go:embed`), not at run time: it says where
releases come from, how to pick the installer and its checksum, when to check (startup, WhatsApp
failures, at most once per hour) and how far the next slot must be before installing. Flow: notify only;
on the user's click, download + checksum, wait for a quiet moment, quit and run the installer silently
(user view in README.md "Updates"). `internal/update` does the GitHub check and download; `internal/app` owns the
install timing (it knows the scheduler).

## Frontend (`frontend/src`)

Layout (chosen from the mockups): fixed left sidebar (Schedules, Status and log, WhatsApp, Trash,
Settings) with the WhatsApp state and the routine toggle at its bottom; pages are a header plus white
cards/tables on a light grey ground. Themes: light, dark and system (follows Windows), all from the
semantic tokens in `app.css`.


| File | Responsibility |
| --- | --- |
| `pages/Schedules.svelte` | list + editor (cadence, recipient, caption, files, next 5 runs, send now) |
| `pages/Status.svelte` | connection, last runs, errors, log viewer |
| `pages/Settings.svelte` | error number, autostart, keep awake, language, theme, updates |
| `pages/WhatsApp.svelte` | QR pairing, re-pair, logout |
| `pages/Trash.svelte` | deleted schedules: restore, empty |
| `lib/api.ts` | calls to the bindings, error code mapping, null normalization |
| `lib/i18n.svelte.ts` | `t()` over `locales/*.json`, current language |
| `app.css` | Tailwind 4 tokens and `@utility` classes |
| `e2e/mock/` | fake backend for Playwright |
