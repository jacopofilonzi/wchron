// The only module that talks to the Go bindings. It normalizes what Go sends
// (nil slices/maps arrive as null) and turns failures into AppError values
// whose `code` the UI maps to `error.<code>` in the catalog.
import { Events } from "@wailsio/runtime";
import { Service, type Snapshot } from "../../bindings/github.com/jacopofilonzi/wchron/internal/app";
import type { Theme } from "../../bindings/github.com/jacopofilonzi/wchron/internal/config";
import type { Lang } from "../../bindings/github.com/jacopofilonzi/wchron/internal/i18n";
import type { Entry as LogEntry } from "../../bindings/github.com/jacopofilonzi/wchron/internal/logging";

export type { Snapshot, Theme, Lang, LogEntry };
export { ChannelsState } from "../../bindings/github.com/jacopofilonzi/wchron/internal/app";
export { Theme as Themes } from "../../bindings/github.com/jacopofilonzi/wchron/internal/config";
export { Lang as Langs } from "../../bindings/github.com/jacopofilonzi/wchron/internal/i18n";

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

type Result<T> = { ok: true; value: T } | { ok: false; error: AppError };

async function call<T>(fn: () => Promise<T>): Promise<Result<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (err) {
    return { ok: false, error: toAppError(err) };
  }
}

export const api = {
  state: () => call(() => Service.State()),
  setRoutineActive: (on: boolean) => call(() => Service.SetRoutineActive(on)),
  setKeepAwake: (on: boolean) => call(() => Service.SetKeepAwake(on)),
  setKeepAwakeDefault: (on: boolean) => call(() => Service.SetKeepAwakeDefault(on)),
  setAutostart: (on: boolean) => call(() => Service.SetAutostart(on)),
  setTheme: (t: Theme) => call(() => Service.SetTheme(t)),
  setLanguage: (l: Lang) => call(() => Service.SetLanguage(l)),
  /** Most recent log entries, newest first; level is the minimum ("" for all), limit 0 for the default. */
  logs: (level: string, limit: number) =>
    call(async (): Promise<LogEntry[]> =>
      ((await Service.Logs(level, limit)) ?? []).map((e) => ({ ...e, attrs: e.attrs ?? {} })),
    ),
  /** A token template rendered for now in the current language; a `token.*` error carries `pos` (1-based character). */
  previewTokens: (template: string) => call(() => Service.PreviewTokens(template)),
};

/** Subscribes to every state change pushed by Go (tray or window); returns the unsubscribe function. */
export function onState(fn: (s: Snapshot) => void): () => void {
  return Events.On("app:state", (ev) => fn(ev.data));
}
