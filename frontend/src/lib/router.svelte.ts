// A minimal hash router: four pages, the schedule editor under
// #/schedules/<id> (or #/schedules/new). Unknown hashes fall back to the
// first page.
//
// Leave guard: a page with unsaved changes (the editor) sets one with
// setLeaveGuard. A navigation away (sidebar link, back) is then put on hold:
// the hash goes back to where it was and `pending` holds the target, so the
// page can ask for a confirmation and call proceed() or stay().
export const pages = ["schedules", "status", "channels", "settings"] as const;
export type Page = (typeof pages)[number];

interface Route {
  page: Page;
  /** The part after the page: a schedule id or "new"; "" for none. */
  param: string;
}

function parse(hash: string): Route {
  const [name = "", param = ""] = hash.replace(/^#\/?/, "").split("/");
  return (pages as readonly string[]).includes(name) ? { page: name as Page, param: decodeURIComponent(param) } : { page: "schedules", param: "" };
}

export const route = $state<Route>(parse(location.hash));

/** A navigation held by the leave guard; null when none. */
export const leave = $state<{ pending: string | null }>({ pending: null });

let guard: (() => boolean) | null = null;
let current = location.hash;

/** Sets (or clears, with null) the function that says whether leaving needs a confirmation. */
export function setLeaveGuard(fn: (() => boolean) | null): void {
  guard = fn;
  if (!fn) leave.pending = null;
}

/** Goes on to the held navigation, without asking again. */
export function proceed(): void {
  const to = leave.pending;
  guard = null;
  leave.pending = null;
  if (to !== null) location.hash = to;
}

/** Cancels the held navigation. */
export function stay(): void {
  leave.pending = null;
}

/** Navigates (through the guard, like a link). */
export function go(hash: string): void {
  location.hash = hash;
}

export function startRouter(): () => void {
  const onHash = () => {
    const to = location.hash;
    if (to === current) return;
    if (guard?.()) {
      leave.pending = to;
      history.replaceState(null, "", current || "#/schedules");
      return;
    }
    current = to;
    const r = parse(to);
    route.page = r.page;
    route.param = r.param;
  };
  window.addEventListener("hashchange", onHash);
  return () => window.removeEventListener("hashchange", onHash);
}

export function href(p: Page, param = ""): string {
  return param ? `#/${p}/${encodeURIComponent(param)}` : `#/${p}`;
}
