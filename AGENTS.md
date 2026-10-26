# AGENTS.md

Guidelines for AI coding agents (and people) working in this repository.
What lives where and how the pieces fit together is in [STRUCTURE.md](STRUCTURE.md): read it first,
instead of re-reading the whole repo. What the app does, as the user sees it, is in [README.md](README.md);
every error code the user can see is in [ERRORS.md](ERRORS.md).

**Temporary: `REFERENCE.md`** (local, git-ignored, never committed) holds the detailed design rules of the
packages not written yet (scheduler, tokens, resolver, processors, pipeline, wa, notify, update).
Read the section of the package you're implementing. When the package lands, move its section
into the package doc comment (and its tests), delete it from `REFERENCE.md`, and delete the file once it's
empty.

These guidelines come from a shared template: https://github.com/jacopofilonzi/agents-guideline.
See "Updating these guidelines" below to align them with the latest version.

## What it is

wchron: a tray desktop app that sends files (optionally Excel workbooks reduced to selected sheets) to a
WhatsApp number or group at scheduled times, through a whatsmeow linked-device session. Stack: Go 1.27 +
Wails 3 (beta), Svelte 5 + TypeScript + Vite + Tailwind 4. Targets: Windows 10/11 only for now; OS-specific
code stays behind interfaces so Linux/macOS can be added later. UI and backend texts in Italian (default)
and English. One process, one binary, single instance (a second launch focuses the window); starts at
login, minimized to the tray.

Non-goals: receiving or reading WhatsApp messages (beyond session health and the group list), bulk or
marketing sending, embedding WhatsApp Web, Linux/macOS builds (keep the code ready, don't ship them).
Don't add features outside README.md / REFERENCE.md without asking; if something there turns out to be
infeasible (e.g. a Wails 3 tray limitation), propose the alternative and ask before deviating.

## Workflow

- **Non-trivial changes start with a plan.** For a feature, refactor, new dependency or anything touching
  several files: read what you need, then present the plan (what changes, which files, trade-offs, open
  questions) and wait for my approval before editing. Small, obvious fixes can go straight in. If the plan
  changes significantly along the way, stop and ask again.
- **Big changes go to one agent, small ones you do yourself.** The goal is to save the main conversation's
  context: the agent does the heavy reading and editing in its own context. Don't spawn sub-agents for
  everything. For a big change, once I've approved the plan, write it out in full (goal, steps, files it
  owns, constraints, how to verify) and launch a single agent to carry it out following that plan; it
  answers with a short report (what changed, which files, `make check` result, open issues), not file
  dumps. Then check it from the diff and `make check` instead of re-reading everything it read. Small
  changes you make directly in the main conversation, without agents.
- **Propose an architecture change when something outgrows its start.** A part that began small and
  simple may keep gaining responsibilities. When it starts to strain its original approach (state wired
  by hand, duplicated markup or logic, ever-growing glue code), stop and propose moving to a better fit,
  with trade-offs and a migration path, instead of piling more onto the simple version. Don't migrate
  without my approval.
- When something is genuinely my decision (UX, user-visible naming, data formats, breaking changes), ask
  instead of guessing. Otherwise pick the sensible default and say what you picked.
- Run commands from the repo root through the Makefile (`make help` lists them). A new recurring command
  goes into the Makefile too. The Makefile wraps the Wails `Taskfile.yml` (`wails3 task …`, `wails3 dev`):
  build logic stays in the Taskfile, the Makefile is the entry point.
- **While developing, use the framework's dev tools** (`make dev`, i.e. `wails3 dev` with hot reload)
  instead of building and launching the app after every change. Build and run the real artifact (exe,
  installer) only when I say the dev tools don't pick up a specific change, and only for that check; then
  go back to the dev server.
- **Always run `make check` before declaring work done.** It must pass with zero errors and zero warnings.
  If you couldn't run something (a live WhatsApp session, a platform you're not on), say so instead of
  claiming it works.
- Makefile recipes must work in both `sh` and `cmd.exe` (GNU make on Windows falls back to `cmd.exe`): only
  `cd dir && command`, no inline `VAR=x cmd`, no `rm`/`cp`/`mkdir -p`; use `node -e` or `go run` for file
  operations.
- **Env vars are only development overrides.** The real configuration is `config.json`, edited from the
  GUI. Env vars exist to point a dev run at a separate environment (data dir, log level…); the installed
  app needs none. Every new env var goes in `internal/devenv` **and** is documented in `.env.example`. The
  Makefile loads `.env` (`-include .env` + `export`), so `make dev` picks it up. Never commit `.env`.
- **Never cause real side effects without asking**: messages to real users, writes to external services.
  Tests use fakes (`sender.Fake`, fake clock, fake WhatsApp client) and fixtures; live checks are read-only.
  When running the app locally for a check, start from an empty data directory (`WCHRON_DATA_DIR` in
  `.env`) unless told otherwise, and stop it afterwards. In this project these act for real:
  - every WhatsApp send: scheduled runs (`pipeline.Run`), the "send now" button, the message to the error
    notification number (`notify.WhatsApp`);
  - QR pairing, re-pair and logout (they create or remove a linked device on the WhatsApp account);
  - the "is on WhatsApp" number check and the group list (read-only, but they hit WhatsApp's servers:
    only with a test session);
  - the update check/download (reads GitHub releases) and the installer it launches;
  - autostart (writes the user's `Run` registry key) and keep-awake (`SetThreadExecutionState`).
- **Development never uses production credentials or services**: dev runs use their own data dir, so their
  own WhatsApp session paired with a test number, never the personal session of the installed app. Test
  recipients are numbers/groups you own.

### Commits

- Small, step-by-step commits as the work progresses, never a single commit at the end. Conventional
  prefixes: `feat:`, `fix:`, `refactor:`, `chore:`, `docs:`, `test:`.
- **Exception, experiments:** while we're trying things out (styling, layouts, alternative approaches),
  don't commit. Commit only once I've picked the version to keep.
- Mockups and design alternatives are never committed: they only serve to choose; the chosen one lives in the code.
- Push, tag and release only when I ask.

### Keep the agent docs in sync, in the same change

`STRUCTURE.md`, `ERRORS.md` and this file exist so agents don't have to re-read the whole repo. When you
add, move, rename or delete a file or package, or change a binding, error code, env var, invariant, data
format or workflow command, update `STRUCTURE.md` / `ERRORS.md` / `AGENTS.md` (and `README.md` for
user-facing changes) in the same commit or right after. Before declaring work done, check that every path,
name and value they mention for the areas you touched still matches the code.

### Updating these guidelines

When I ask you to check for new guidelines, fetch the latest template from
https://github.com/jacopofilonzi/agents-guideline (raw files:
`https://raw.githubusercontent.com/jacopofilonzi/agents-guideline/main/{AGENTS,CLAUDE,STRUCTURE}.md`)
and align this project's copies with it:

- Bring in new and changed generic rules (workflow, commits, code, testing…) and drop the ones removed
  upstream, without asking.
- Keep everything project-specific: filled-in placeholders, invariants, recipes, pitfalls, the whole
  content of `STRUCTURE.md`, and rules this project added or deliberately changed.
- Ask only when an upstream change conflicts with the project's current style or guidelines (e.g. it
  contradicts a project-specific rule or a convention the code follows); list each conflict with both versions.
- Then summarize what changed and commit it as `docs: sync agent guidelines`.

Deliberate project changes to the template (keep them on sync): env vars are dev-only overrides; no
Docker; CI runs only on a `v*` tag push and only builds and releases (checks run locally before the push); error codes are listed in `ERRORS.md`;
i18n lives in one shared catalog used by both Go and the frontend.

## Invariants — don't break these

- **The schedule's recipient never receives anything but the configured files and captions.** Errors and
  notices go to the desktop and, if set, to the error notification number. The sender is never used for
  anything else.
- **Missed slots are never recovered.** No catch-up bursts, no retry loops: at most one short retry on a
  network failure, then skip and log. Slots are computed on the local wall clock of the system time zone.
- **The original files are never modified.** Processors work on a temp copy, deleted also on failure.
- The core (`internal/` except `internal/platform`, `internal/tray` and `main.go`) never imports
  Windows-specific packages and never branches on `runtime.GOOS`.
- Session keys and the WhatsApp store are never logged; phone numbers in logs are masked except the last 3 digits.
- **Upgrading never loses data or stops the service.** Durable data is `config.json` (schedules, settings,
  trash), `state.json` (last runs) and `session.db` (whatsmeow). IDs and formats stay backward compatible.
  Format changes are migration steps appended to `internal/config/migrations.go` (and
  `internal/state/migrations.go`), never edits to one already released (pushed to `main`); each bumps
  `schema_version`. A migration that touches existing data gets a test that starts from a file at the
  previous version with realistic data. Before migrating, or when the app version changed since the last
  run, the app copies the data files to `backups/<timestamp>-v<old version>/` (`session.db` via
  `VACUUM INTO`) and refuses to start (`storage.backup_failed`) if the backup fails. A file with a
  `schema_version` newer than the app is never overwritten (`config.newer_version`). Anything the user must
  do before updating goes in the README upgrade notes.
- Writes to `config.json` and `state.json` are atomic (temp file + rename).
- Every error the user can see has a code listed in `ERRORS.md` and a text in both languages; a test fails
  if the code registry, the catalogs and `ERRORS.md` disagree.

## Code

- Everything is written in English (code, comments, commits, docs), except the language packs.
- Keep modules small and focused: one package per area; split a file when it grows several responsibilities.
- Comments explain *why*, not *what*. Put the design of a non-trivial package in its package doc comment
  (formats, invariants, data layout).
- Never ignore errors: wrap them with context (`fmt.Errorf("…: %w", err)`). No panics, no unchecked type
  assertions on external data (files, WhatsApp events, user input).
- Errors that reach the user are `apperr.Error` with a stable machine-readable `code` (from ERRORS.md),
  optional `params` (file name, sheet name…) and an English message for logs. The UI and the backend
  notifications map the code to the i18n text. User-facing text never comes from an error's message.
- Use the project's logger (`log/slog` through `internal/logging`), never `fmt.Println`/`console.log` in
  core logic.
- OS-specific code is isolated in `internal/platform` (`_windows.go` files, `_other.go` stubs so the rest
  still compiles elsewhere); no branching on the OS elsewhere. Build paths with `filepath.Join`.
- Reuse shared clients and connections (one HTTP client, one whatsmeow client) instead of creating them per call.
- Time always comes from an injected `clock.Clock`, never `time.Now()` in the scheduler, pipeline or token engine.
- **Dependencies are recent majors whose APIs differ from most online examples:** check the docs for the
  installed version before using an API. Ask before adding a new dependency.
  - Wails `v3.0.0-beta.x` (pre-stable, pinned in `go.mod`; the `wails3` CLI must match it). Don't run
    `wails3 update build-assets`: it overwrites the customized files in `build/`.
  - whatsmeow: no tagged releases, pinned to a commit; its API changes often, read the source at that commit.
  - `modernc.org/sqlite` (no CGO): the driver name is `sqlite`, not `sqlite3`.
  - excelize v2: reads cached formula values, never recalculates.
  - Svelte 5, Vite, Tailwind 4 (`@tailwindcss/vite`, CSS-first config, no `tailwind.config.js`).
- Deleting user data means moving it to the trash: deleted schedules (and file specs) go to the in-app
  trash in `config.json`, restorable from the GUI, removed only by an explicit "empty trash". Files on disk
  that the app owns go to the Windows recycle bin. Temp copies made by processors are not user data.

### UI

- Every UI string goes through i18n, with both the English and the Italian key (`it` is the default); the
  type check must catch a missing key.
- Svelte 5 runes only (`$state`, `$derived`, `$effect`, `$props`): no legacy stores or `export let`.
  Typed state is `$state<T>(…)` (an annotated `let x: T = $state(…)` narrows to the initial value).
  Don't name a variable `state` (it clashes with the rune).
- Tailwind 4: reusable classes are `@utility` in the global CSS. Colours are semantic tokens defined there
  (`bg-surface`, `text-muted`…), never raw colours in the markup. Every token has a light and a dark
  value; the theme (`system` | `light` | `dark`) switches them, so a component never styles dark mode itself.
- Models come from the Wails generated bindings (`frontend/bindings`): never redeclare them in the UI.
  Every call goes through `frontend/src/lib/api.ts` (error code mapping, null slices → empty arrays).
- No external scripts from CDNs; libraries are npm packages, and heavy ones are loaded lazily.

### i18n

- One catalog for Go and the frontend: `locales/it.json` and `locales/en.json`, embedded in Go
  (`locales/embed.go`) and imported by the frontend. Keys are namespaced: `ui.*` (frontend only),
  `tray.*`, `notify.*`, `wa.*` (texts sent over WhatsApp to the error number), `error.<code>`.
- Placeholders are `{name}`, filled from the error `params` or the caller.
- `en.json` is the reference set of keys: the frontend types `it` as `typeof en`, a Go test checks that both
  files have the same keys and that every code in `internal/apperr` has an `error.<code>` key.

## Testing

- Unit tests live next to the code. Test parsing against fixtures, never live requests; live checks are a
  separate opt-in command (`make smoke`, read-only: connection state and group list of the dev session).
- Required coverage: token engine (Italian names, offsets, escaping), scheduler (fake clock: interval,
  fixed times, window, days, jitter, skipped slots, DST both ways), Excel processor (fixture workbooks:
  formulas, missing sheet, locked file, no cached values), config/state migrations.
- Fixture workbooks are synthetic (generated or hand-made), never copies of real user files. Before
  committing anything taken from a real response, replace names, phone numbers, JIDs and any token.
- End-to-end checks: the frontend runs in Playwright against a fake backend (`frontend/e2e/mock`), no
  Wails, no disk; install `playwright-core` in a scratch dir and use the system Chrome/Edge, unless the
  project's own e2e setup is in place.

## Versions and releases

- Versions are `x.y.z`: `x` major (only when I decide), `y` feature, `z` fix. A feature bumps `y` and
  resets `z`. Development starts at `0.1.0`.
- The version lives in `build/config.yml`, `build/windows/info.json`, `build/windows/nsis/wails_tools.nsh`,
  `build/windows/wails.exe.manifest`, `internal/version/version.go`, `frontend/package.json` (+ lock):
  change it only with `make version V=x.y.z` (`scripts/set-version.mjs`), which updates them all and checks
  that the old version is left nowhere.
- Every completed feature or fix ends with `make version` and a `chore: version x.y.z` commit. Tags and
  releases only when I ask.
- CI (`.github/workflows/release.yml`) runs only on a `vX.Y.Z` tag push, never on a plain push and no
  manual trigger. It doesn't run the checks: `make check` must pass locally before pushing the tag. It
  only verifies that the tag matches `build/config.yml`, builds the per-user NSIS installer and creates the
  GitHub release with the installer and its `.sha256` attached.
- `make package` puts the exe and the installer side by side in `bin/` (`wchron.exe`,
  `wchron-amd64-installer.exe`). The installer is per-user only (`$LOCALAPPDATA\Programs\wchron`, no UAC),
  never system-wide.
- The build-time update settings (repo, asset names, channel) are in `update.config.json`, embedded at
  compile time. See STRUCTURE.md "Updates".

## Adding a field to a model

1. Change the Go type (`internal/config` or `internal/state`): the compiler then points at the defaults,
   validation and normalization to update.
2. If old files need it filled or reshaped: append a migration step and bump `schema_version`, with a test
   from the previous version.
3. Regenerate the bindings (`make bindings`), then update every view that shows or edits it, and the fake
   backend in `frontend/e2e/mock`.
4. Tests (valid value, invalid value, field missing in old files); update `STRUCTURE.md`.

## Adding a processor

1. Create `internal/processor/<name>/` implementing `processor.Processor` (`Type`, `OptionsSchema`,
   `Validate`, `Process`). `Process` works on a temp copy and returns a cleanup func.
2. Register it in `internal/processor/registry.go`. The GUI renders its options from `OptionsSchema`.
3. Add its error codes to `internal/apperr`, `ERRORS.md` and both catalogs; labels for its options go in
   `locales/*.json` under `ui.processor.<name>.*`.
4. Tests with synthetic fixtures in `internal/processor/<name>/testdata`.

## Adding an error code

1. Add the constant to `internal/apperr/codes.go` (dotted lowercase, `area.reason`).
2. Add `error.<code>` to `locales/en.json` and `locales/it.json`, with its `{params}`.
3. Add the row to `ERRORS.md` (where it shows up, params, what the user can do). `make check` fails if one
   of the three is missing.

## Known pitfalls

- The main dev machine is Windows 11; shells are PowerShell and Git Bash. Commands and scripts must work there.
- **Git Bash rewrites env values that look like paths** (`BASE_PATH=/x` becomes `C:/Program Files/Git/x`):
  prefix the command with `MSYS2_ENV_CONV_EXCL='*'`, or set the value in `.env`.
- A running `.exe` locks its binary on Windows: stop it (from the tray, Quit) before rebuilding or
  reinstalling. Single instance means a second launch only focuses the first.
- If `go env GOARCH` is `386`, Wails won't run: build and test with `ARCH=amd64` / `GOARCH=amd64`.
- The time zone is always the system one (`time.Local`); there is no setting. The scheduler takes the
  `*time.Location` as a parameter so DST tests can pass `Europe/Rome` explicitly: on Windows,
  `time.LoadLocation` only works where Go is installed, so test files import `time/tzdata` (tests only,
  never the app).
- Go nil slices and maps reach the frontend as `null`: normalize them in `frontend/src/lib/api.ts`.
- `frontend/bindings` (`make bindings`) and `frontend/dist` are generated and git-ignored. `main.go` embeds
  `all:frontend/dist`, which must hold a file for `go vet`/`go test` to compile: the Makefile writes a
  `.keep` there when it's empty.
- Wails single instance: `application.New` exits a second launch right away (after notifying the first),
  so nothing with side effects may run before it in `main.go`.
- npm 12 refuses `npm install --package-lock-only` here (optional tarballs): `set-version.mjs` edits the
  lock's own version entries directly.
- `wails3 generate icons` always writes macOS files too: `build/windows/icon.ico` is a committed asset,
  not regenerated by the build.
- whatsmeow logs to its own logger interface: adapt it to slog in `internal/wa`, never let it print to stdout.
