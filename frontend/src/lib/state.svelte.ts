// The app state mirrored from Go. Go is the source of truth: the UI applies
// the snapshot returned by each call and every `app:state` event, so changes
// made from the tray show up here too.
import { api, onState, Themes, type Snapshot } from "./api";
import { setLang } from "./i18n.svelte";

export const app = $state<{ s: Snapshot | null }>({ s: null });

/** Shows a snapshot returned by a call or pushed by Go. */
export function applySnapshot(s: Snapshot): void {
  app.s = s;
  setLang(s.language);
  document.documentElement.dataset.theme = s.theme || Themes.ThemeSystem;
}

/** Loads the state and follows the changes; returns the cleanup function. */
export function startState(): () => void {
  const off = onState(applySnapshot);
  void api.state().then((r) => {
    if (r.ok) applySnapshot(r.value);
  });
  return off;
}

