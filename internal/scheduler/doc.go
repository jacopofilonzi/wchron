// Package scheduler turns the schedules' cadences into slots and fires them
// at the right time: on the local wall clock, through sleep/resume, clock
// changes and DST, never catching up.
//
// # Cadence → slots
//
// Slots are computed on the wall clock of a *time.Location passed as a
// parameter (the app passes time.Local: there is no time zone setting; tests
// pass Europe/Rome), building times with time.Date, never by adding
// durations. Each schedule has a mode:
//
//   - Interval: hours + minutes, 1 minute to 24h. Every calendar day the slots
//     restart from the anchor (default 00:00): anchor + k·interval, up to the
//     last one before the next midnight, so there is no drift even when the
//     interval doesn't divide the day. An optional window {start, end} keeps
//     only the slots inside it, both ends included (every hour 08:00–20:00
//     fires at 20:00). start == end is invalid. A window may cross midnight
//     (22:00–02:00): the part after midnight belongs to the day the window
//     opened, so the active days are checked on the opening day, while its
//     slots are still the next calendar day's anchor + k·interval. A window
//     with no slot inside is invalid (cadence.window_empty).
//   - Fixed: a list of HH:MM times, no duplicates; the window is not used.
//
// Active days Mon–Sun, at least one (schedule.no_days). A slot's day is its
// calendar day, except after midnight in an overnight window (above).
// Validate holds the checks (cadence.*, schedule.no_days): config.Validate is
// structural only, so a schedule broken by a hand edit is ignored here with a
// warning and never stops the app; the editor (milestone 8) calls Validate
// on save. Next gives the next n slots for the editor preview.
//
// DST: a slot whose wall-clock time doesn't exist (spring gap) is skipped
// with slot.dst_gap. A slot in the repeated autumn hour has two instants: it
// fires once, at the first; the second is skipped with slot.dst_repeat
// (occurrences tells the two apart, since time.Date picks one silently).
// state's last_slot is stored as an RFC 3339 instant with its offset, so the
// two occurrences differ: a slot whose wall clock equals last_slot never runs
// again (a restart between the two occurrences, a clock set back).
//
// # Firing
//
// One goroutine (loop). It never trusts long timers: it sleeps until the
// next due event or MaxSleep (30 s), whichever comes first, and reads the
// injected clock again at every wake, so sleep/hibernate, clock jumps and DST
// are seen within 30 s. There is no time.Now() in this package.
//
//   - Jitter: a random 0–2 min delay drawn once per slot when it becomes the
//     schedule's next event, applied only to that start; slots stay anchored.
//   - Tolerance: a slot starts only if now − slot ≤ 5 min. The runner gets
//     the deadline (slot + 5 min) and re-checks it before its first send
//     (slot.late).
//   - Missed slots are never recovered: the slots found more than 5 min in
//     the past at a wake (PC asleep, clock moved forward) are recorded as ONE
//     slot.missed per schedule {schedule, count, from, to} in the log and as
//     the schedule's last result. Slots that passed while the app was closed
//     are not reported: at start (and for a new or edited schedule) the
//     slots are computed from now.
//   - Routine paused (Store.RoutineActive false): slots don't run; one
//     slot.paused per schedule per paused stretch {schedule, count, from,
//     to}, updated in state at each slot and logged when the stretch ends.
//   - Different schedules run in parallel, each run on its own goroutine. A
//     schedule never overlaps itself: a slot that comes while its previous
//     run is still going is skipped (slot.overlap). Serializing the sends of
//     one channel across schedules (pauses between sends) belongs to the
//     pipeline (milestone 6), not here.
//   - Disabled schedules and invalid cadences are ignored. Reload takes the
//     whole schedule list after any config change: an unchanged cadence keeps
//     its position, a new or edited one starts from now (no back-firing).
//   - A run records last_slot and last_run_at when it starts and
//     last_result/last_error when the Runner returns. Skips set last_result
//     "skipped" with the reason in last_error and leave last_slot alone.
//   - The earliest upcoming run (NextRun) goes to OnNext whenever it
//     changes; the app puts it in its Snapshot (tray status line, app:state).
//   - Stop cancels the waits and the runs' context and waits briefly for
//     the runs in progress.
//
// Until the pipeline exists, the app runs DryRunner, which only logs.
//
// Known limit: Go reads the system time zone once at process start, so a
// time zone change in Windows takes effect after a restart of the app.
package scheduler
