// The only module that talks to the Go bindings. It normalizes what Go sends
// (nil slices/maps arrive as null) and turns failures into AppError values
// whose `code` the UI maps to `error.<code>` in the catalog.
import { Events } from "@wailsio/runtime";
import {
  Service,
  type FieldProblem,
  type FilePreview,
  type PreviewFile,
  type RunEvent,
  type RunRecord,
  type SaveResult,
  type ScheduleSummary,
  type Snapshot,
} from "../../bindings/github.com/jacopofilonzi/wchron/internal/app";
import type { Ref } from "../../bindings/github.com/jacopofilonzi/wchron/internal/apperr";
import type {
  Cadence,
  FileSpec,
  MatchMode,
  Recipient,
  Schedule,
  Theme,
  Weekday,
} from "../../bindings/github.com/jacopofilonzi/wchron/internal/config";
import type { Field as ProcessorField, Schema as ProcessorSchema } from "../../bindings/github.com/jacopofilonzi/wchron/internal/processor";
import type { Lang } from "../../bindings/github.com/jacopofilonzi/wchron/internal/i18n";
import type { Entry as LogEntry } from "../../bindings/github.com/jacopofilonzi/wchron/internal/logging";
import type {
  ChannelInfo,
  DeleteResult,
  Field,
  Info as ProviderInfo,
  PairingEvent,
  Status as ChannelStatus,
  Target,
  TargetCheck,
  Usage,
} from "../../bindings/github.com/jacopofilonzi/wchron/internal/provider";

export type { Snapshot, Theme, Lang, LogEntry, Cadence, RunEvent };
export type { Schedule, FileSpec, Recipient, Weekday, ScheduleSummary, FieldProblem, FilePreview, PreviewFile, RunRecord };
export type { ProcessorSchema, ProcessorField };
export { CadenceMode, MatchMode, Verified } from "../../bindings/github.com/jacopofilonzi/wchron/internal/config";
export type { ChannelInfo, ChannelStatus, DeleteResult, Field, ProviderInfo, PairingEvent, Target, TargetCheck, Usage };
export { ChannelsState } from "../../bindings/github.com/jacopofilonzi/wchron/internal/app";
export { FieldKind as ProcessorFieldKind } from "../../bindings/github.com/jacopofilonzi/wchron/internal/processor";
export { Theme as Themes } from "../../bindings/github.com/jacopofilonzi/wchron/internal/config";
export { Lang as Langs } from "../../bindings/github.com/jacopofilonzi/wchron/internal/i18n";
export { State as ChannelStates, PairKind } from "../../bindings/github.com/jacopofilonzi/wchron/internal/provider";

/** A user-visible error: `code` from ERRORS.md, `params` for the text placeholders. */
export interface AppError {
  code: string;
  params: Record<string, string>;
}

/** Converts whatever a binding threw into an AppError (`internal` when it has no code). */
export function toAppError(err: unknown): AppError {
  const cause = err instanceof Error ? err.cause : undefined;
  if (cause && typeof cause === "object" && "code" in cause && typeof cause.code === "string") {
    const params = "params" in cause && cause.params && typeof cause.params === "object" ? cause.params : {};
    return { code: cause.code, params: params as Record<string, string> };
  }
  return { code: "internal", params: {} };
}

/** An error carried as data (a channel's status reason, a warning). */
export function fromRef(r: Ref | null | undefined): AppError | null {
  return r ? { code: r.code, params: (r.params ?? {}) as Record<string, string> } : null;
}

type Result<T> = { ok: true; value: T } | { ok: false; error: AppError };

async function call<T>(fn: () => Promise<T>): Promise<Result<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (err) {
    return { ok: false, error: toAppError(err) };
  }
}

function normRef(r: Ref | null): Ref | null {
  return r ? { ...r, params: r.params ?? {} } : null;
}

function normChannel(c: ChannelInfo): ChannelInfo {
  return { ...c, status: { ...c.status, error: normRef(c.status.error) } };
}

function normSnapshot(s: Snapshot): Snapshot {
  return { ...s, channel_list: (s.channel_list ?? []).map(normChannel) };
}

function normFields(fs: Field[] | null): Field[] {
  return (fs ?? []).map((f) => ({ ...f, options: f.options ?? [] }));
}

/** A problem under one field of a form (`field` as addressed by Go, e.g. `files.<id>.path_pattern`). */
export interface Problem {
  field: string;
  error: AppError;
}

function normProblems(ps: FieldProblem[] | null): Problem[] {
  return (ps ?? []).map((p) => ({ field: p.field, error: { code: p.code, params: (p.params ?? {}) as Record<string, string> } }));
}

function normRecipients(rs: Recipient[] | null): Recipient[] {
  return rs ?? [];
}

/** Go nil slices arrive as null: every list of a schedule becomes an array. */
export function normSchedule(s: Schedule): Schedule {
  return {
    ...s,
    cadence: { ...s.cadence, times: s.cadence.times ?? [], days: s.cadence.days ?? [] },
    recipients: normRecipients(s.recipients),
    files: (s.files ?? []).map((f) => ({ ...f, processor_options: f.processor_options ?? {} })),
  };
}

function normSummary(s: ScheduleSummary): ScheduleSummary {
  return { ...s, schedule: normSchedule(s.schedule), last_error: normRef(s.last_error), invalid: normRef(s.invalid) };
}

/** The outcome of saving a schedule: the saved one, or the problems under the fields. */
export interface Saved {
  schedule: Schedule | null;
  problems: Problem[];
}

const snap = (fn: () => Promise<Snapshot>) => call(async () => normSnapshot(await fn()));
const channel = (fn: () => Promise<ChannelInfo>) => call(async () => normChannel(await fn()));

export const api = {
  state: () => snap(() => Service.State()),
  setRoutineActive: (on: boolean) => snap(() => Service.SetRoutineActive(on)),
  setKeepAwake: (on: boolean) => snap(() => Service.SetKeepAwake(on)),
  setKeepAwakeDefault: (on: boolean) => snap(() => Service.SetKeepAwakeDefault(on)),
  setAutostart: (on: boolean) => snap(() => Service.SetAutostart(on)),
  setTheme: (t: Theme) => snap(() => Service.SetTheme(t)),
  setLanguage: (l: Lang) => snap(() => Service.SetLanguage(l)),
  /** Most recent log entries, newest first; level is the minimum ("" for all), limit 0 for the default. */
  logs: (level: string, limit: number) =>
    call(async (): Promise<LogEntry[]> =>
      ((await Service.Logs(level, limit)) ?? []).map((e) => ({ ...e, attrs: e.attrs ?? {} })),
    ),
  /** A token template rendered for now in the current language; a `token.*` error carries `pos` (1-based character). */
  previewTokens: (template: string) => call(() => Service.PreviewTokens(template)),
  /** The next n run times of a cadence from now (RFC 3339, local offset); a `cadence.*` or `schedule.no_days` error if invalid. */
  previewRuns: (cadence: Cadence, n: number) =>
    call(async (): Promise<string[]> => (await Service.PreviewRuns(cadence, n)) ?? []),
  /** The sheet names (workbook order) of the workbook a path pattern resolves to now (newest match), for "Values only"; `filespec.*`, `token.*`, `file.*`, `xlsx.not_xlsx`, `xlsx.open_failed` on error. */
  readSheets: (pathPattern: string) =>
    call(async (): Promise<string[]> => (await Service.ReadSheets(pathPattern)) ?? []),

  /** The available providers with their capabilities and field schemas. */
  providers: () =>
    call(async (): Promise<ProviderInfo[]> =>
      ((await Service.Providers()) ?? []).map((p) => ({
        ...p,
        settings_schema: normFields(p.settings_schema),
        target_schema: normFields(p.target_schema),
      })),
    ),
  listChannels: () => call(async (): Promise<ChannelInfo[]> => ((await Service.ListChannels()) ?? []).map(normChannel)),
  createChannel: (provider: string, name: string) => channel(() => Service.CreateChannel(provider, name)),
  renameChannel: (id: string, name: string) => channel(() => Service.RenameChannel(id, name)),
  /** What deleting the channel affects, for the confirmation dialog. */
  channelUsage: (id: string) =>
    call(async (): Promise<Usage> => {
      const u = await Service.ChannelUsage(id);
      return { ...u, schedules: u.schedules ?? [] };
    }),
  /** Acts for real: logs the channel out where possible, then deletes it and its recipients. */
  deleteChannel: (id: string) => call(() => Service.DeleteChannel(id)),
  /** Acts for real: starts a pairing (a paired channel is logged out first); steps arrive through onPairing. */
  startPairing: (id: string) => channel(() => Service.StartPairing(id)),
  cancelPairing: (id: string) => call(() => Service.CancelPairing(id)),
  /** Acts for real: removes the linked device from the account; the channel stays. */
  logout: (id: string) => channel(() => Service.Logout(id)),
  /** Normalizes and checks a target; offline it comes back unverified with a `recipient.check_unavailable` warning. */
  validateTarget: (channelID: string, target: unknown) =>
    call(async (): Promise<TargetCheck> => {
      const c = await Service.ValidateTarget(channelID, target);
      return { ...c, warning: normRef(c.warning) };
    }),
  listTargets: (channelID: string) => call(async (): Promise<Target[]> => (await Service.ListTargets(channelID)) ?? []),

  /** Every schedule with its last result and next run. */
  listSchedules: () => call(async (): Promise<ScheduleSummary[]> => ((await Service.ListSchedules()) ?? []).map(normSummary)),
  getSchedule: (id: string) => call(async () => normSchedule(await Service.GetSchedule(id))),
  /** The defaults of a new schedule (not saved; empty id). */
  newSchedule: () => call(async () => normSchedule(await Service.NewSchedule())),
  /** A new file spec with its id. */
  newFileSpec: () => call(async (): Promise<FileSpec> => {
    const f = await Service.NewFileSpec();
    return { ...f, processor_options: f.processor_options ?? {} };
  }),
  newRecipientID: () => call(() => Service.NewRecipientID()),
  /** Validates and saves (an empty id adds it); problems come back under their fields, nothing saved then. */
  saveSchedule: (s: Schedule) =>
    call(async (): Promise<Saved> => {
      const r: SaveResult = await Service.SaveSchedule(s);
      return { schedule: r.schedule ? normSchedule(r.schedule) : null, problems: normProblems(r.problems) };
    }),
  /** A disabled copy right after the original, named "<name> (copy)". */
  duplicateSchedule: (id: string) => call(async () => normSchedule(await Service.DuplicateSchedule(id))),
  /** Permanent: call only after the confirmation dialog. */
  deleteSchedule: (id: string) => call(() => Service.DeleteSchedule(id)),
  setScheduleEnabled: (id: string, on: boolean) => call(async () => normSchedule(await Service.SetScheduleEnabled(id, on))),
  processorSchemas: () =>
    call(async (): Promise<ProcessorSchema[]> =>
      ((await Service.ProcessorSchemas()) ?? []).map((p) => ({ ...p, extensions: p.extensions ?? [], options: p.options ?? [] })),
    ),
  /** What a pattern resolves to now; a problem comes back in `error`, never as a failure. */
  previewFiles: (pattern: string, match: MatchMode) =>
    call(async (): Promise<FilePreview> => {
      const p = await Service.PreviewFiles(pattern, match);
      return { ...p, files: p.files ?? [], error: normRef(p.error) };
    }),
  /** The names the recipients would see now (same rules as a run: extension, numbering). */
  previewSendName: (template: string, pattern: string, match: MatchMode, processor: string) =>
    call(async (): Promise<string[]> => (await Service.PreviewSendName(template, pattern, match, processor)) ?? []),
  errorRecipients: () => call(async (): Promise<Recipient[]> => normRecipients(await Service.ErrorRecipients())),
  /** Replaces the error recipients; problems come back under `error_recipients.<id>`, nothing saved then. */
  saveErrorRecipients: (rs: Recipient[]) => call(async (): Promise<Problem[]> => normProblems(await Service.SaveErrorRecipients(rs))),
  dataDir: () => call(() => Service.DataDir()),
  openDataDir: () => call(() => Service.OpenDataDir()),
  openLogsDir: () => call(() => Service.OpenLogsDir()),
  /** When the data was last backed up (RFC 3339), or null. */
  lastBackup: () => call(() => Service.LastBackup()),
  /** The most recent runs and skips from the log, newest first. */
  runHistory: (limit: number) =>
    call(async (): Promise<RunRecord[]> => ((await Service.RunHistory(limit)) ?? []).map((r) => ({ ...r, error: normRef(r.error) }))),

  /** Acts for real: runs a schedule now (no days/window/tolerance); `schedule.busy` if it is already running. The outcome arrives through onRunFinished. */
  sendNow: (scheduleID: string) => call(() => Service.SendNow(scheduleID)),
  /** Clears the tray's error state; the Status page calls it when it opens. */
  ackErrors: () => snap(() => Service.AckErrors()),
};

/** Subscribes to every state change pushed by Go (tray or window); returns the unsubscribe function. */
export function onState(fn: (s: Snapshot) => void): () => void {
  return Events.On("app:state", (ev) => fn(normSnapshot(ev.data)));
}

function normRun(r: RunEvent): RunEvent {
  return { ...r, problems: (r.problems ?? []).map((p) => ({ ...p, params: p.params ?? {} })) };
}

/** Subscribes to the schedule changes (saved, deleted, enabled, recipients removed with a channel). */
export function onSchedulesChanged(fn: () => void): () => void {
  return Events.On("schedules:changed", () => fn());
}

/** Subscribes to the end of every run (status and problems); returns the unsubscribe function. */
export function onRunFinished(fn: (r: RunEvent) => void): () => void {
  return Events.On("run:finished", (ev) => fn(normRun(ev.data)));
}

/** Subscribes to the pairing steps of every channel (QR codes, result); returns the unsubscribe function. */
export function onPairing(fn: (p: PairingEvent) => void): () => void {
  return Events.On("channel:qr", (ev) => fn({ ...ev.data, error: normRef(ev.data.error) }));
}
