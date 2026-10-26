// The only module that talks to the Go bindings. It normalizes what Go sends
// (nil slices/maps arrive as null) and turns failures into AppError values
// whose `code` the UI maps to `error.<code>` in the catalog.
import { Events } from "@wailsio/runtime";
import { Service, type RunEvent, type Snapshot } from "../../bindings/github.com/jacopofilonzi/wchron/internal/app";
import type { Ref } from "../../bindings/github.com/jacopofilonzi/wchron/internal/apperr";
import type { Cadence, Theme } from "../../bindings/github.com/jacopofilonzi/wchron/internal/config";
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
export type { ChannelInfo, ChannelStatus, DeleteResult, Field, ProviderInfo, PairingEvent, Target, TargetCheck, Usage };
export { ChannelsState } from "../../bindings/github.com/jacopofilonzi/wchron/internal/app";
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

/** Subscribes to the end of every run (status and problems); returns the unsubscribe function. */
export function onRunFinished(fn: (r: RunEvent) => void): () => void {
  return Events.On("run:finished", (ev) => fn(normRun(ev.data)));
}

/** Subscribes to the pairing steps of every channel (QR codes, result); returns the unsubscribe function. */
export function onPairing(fn: (p: PairingEvent) => void): () => void {
  return Events.On("channel:qr", (ev) => fn({ ...ev.data, error: normRef(ev.data.error) }));
}
