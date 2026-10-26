// The only module that talks to the Go bindings. It normalizes what Go sends
// (nil slices/maps arrive as null) and turns failures into AppError values
// whose `code` the UI maps to `error.<code>` in the catalog.
import { Events } from "@wailsio/runtime";
import { Service, type Snapshot, type Theme } from "../../bindings/github.com/jacopofilonzi/wchron/internal/app";
import type { Lang } from "../../bindings/github.com/jacopofilonzi/wchron/internal/i18n";

export type { Snapshot, Theme, Lang };
export { Theme as Themes, ChannelsState } from "../../bindings/github.com/jacopofilonzi/wchron/internal/app";
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
  setAutostart: (on: boolean) => call(() => Service.SetAutostart(on)),
  setTheme: (t: Theme) => call(() => Service.SetTheme(t)),
  setLanguage: (l: Lang) => call(() => Service.SetLanguage(l)),
};

/** Subscribes to every state change pushed by Go (tray or window); returns the unsubscribe function. */
export function onState(fn: (s: Snapshot) => void): () => void {
  return Events.On("app:state", (ev) => fn(ev.data));
}
