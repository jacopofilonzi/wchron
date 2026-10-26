// The fake backend's small re-implementations of Go rules the UI shows live:
// date tokens, the next runs of a cadence, the cadence checks, send names.
// Only what the end-to-end tests exercise; Go stays the source of truth.

/** A user-visible error, thrown like a Wails binding error (`cause` = {code, params}). */
export function fail(code: string, params: Record<string, string> = {}): never {
  throw new Error(code, { cause: { code, params } });
}

const months = {
  it: ["gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"],
  en: ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"],
};
const days = {
  it: ["domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato"],
  en: ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"],
};
const pad = (n: number) => String(n).padStart(2, "0");

function format(d: Date, f: string, lang: "it" | "en"): string {
  return f.replace(/YYYY|YY|MMMM|MM|M|DD|D|dddd|HH|mm/g, (k) => {
    switch (k) {
      case "YYYY":
        return String(d.getFullYear());
      case "YY":
        return String(d.getFullYear()).slice(2);
      case "MMMM":
        return months[lang][d.getMonth()]!;
      case "MM":
        return pad(d.getMonth() + 1);
      case "M":
        return String(d.getMonth() + 1);
      case "DD":
        return pad(d.getDate());
      case "D":
        return String(d.getDate());
      case "dddd":
        return days[lang][d.getDay()]!;
      case "HH":
        return pad(d.getHours());
      default:
        return pad(d.getMinutes());
    }
  });
}

/** Renders {date[±Nd]:FORMAT} tokens at now; token.* errors with a 1-based pos. */
export function renderTokens(tpl: string, now: Date, lang: "it" | "en"): string {
  let out = "";
  for (let i = 0; i < tpl.length; i++) {
    const c = tpl[i]!;
    if (c === "{" && tpl[i + 1] === "{") {
      out += "{";
      i++;
      continue;
    }
    if (c === "}" && tpl[i + 1] === "}") {
      out += "}";
      i++;
      continue;
    }
    if (c === "}") fail("token.unexpected_close", { pos: String(i + 1) });
    if (c !== "{") {
      out += c;
      continue;
    }
    const end = tpl.indexOf("}", i);
    if (end < 0) fail("token.unclosed", { pos: String(i + 1) });
    const body = tpl.slice(i + 1, end);
    const m = /^date([+-]\d+d)?:(.+)$/.exec(body);
    if (!m) fail("token.unknown", { pos: String(i + 2), name: body.split(":")[0] ?? "" });
    if (/[QqWw]/.test(m[2]!)) fail("token.format_invalid", { pos: String(i + 7), format: m[2]! });
    const d = new Date(now);
    if (m[1]) d.setDate(d.getDate() + Number(m[1].slice(0, -1)));
    out += format(d, m[2]!, lang);
    i = end;
  }
  return out;
}

export interface Cadence {
  mode: string;
  interval: { hours: number; minutes: number };
  anchor: string;
  window: { start: string; end: string } | null;
  times: string[] | null;
  days: string[] | null;
}

const weekdays = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"];

function hhmm(s: string): number | null {
  const m = /^(\d\d):(\d\d)$/.exec(s);
  if (!m) return null;
  const h = Number(m[1]);
  const mi = Number(m[2]);
  return h > 23 || mi > 59 ? null : h * 60 + mi;
}

/** The day minutes of a cadence, or its cadence.* / schedule.no_days error. */
export function compile(c: Cadence): { minutes: number[]; window: [number, number] | null } {
  let minutes: number[] = [];
  let win: [number, number] | null = null;
  if (c.mode === "interval") {
    const h = Number(c.interval.hours) || 0;
    const mi = Number(c.interval.minutes) || 0;
    const iv = h * 60 + mi;
    if (h < 0 || mi < 0 || mi > 59 || iv < 1 || iv > 1440) fail("cadence.interval_invalid");
    const a = c.anchor === "" ? 0 : hhmm(c.anchor);
    if (a === null) fail("cadence.anchor_invalid", { value: c.anchor });
    for (let m = a; m < 1440; m += iv) minutes.push(m);
    if (c.window) {
      const s = hhmm(c.window.start);
      const e = hhmm(c.window.end);
      if (s === null || e === null || s === e) fail("cadence.window_invalid");
      win = [s, e];
      const inWin = (m: number) => (s < e ? s <= m && m <= e : m >= s || m <= e);
      minutes = minutes.filter(inWin);
      if (minutes.length === 0) fail("cadence.window_empty");
    }
  } else {
    const times = c.times ?? [];
    if (times.length === 0) fail("cadence.no_times");
    for (const t of times) {
      const m = hhmm(t);
      if (m === null) fail("cadence.time_invalid", { value: t });
      if (minutes.includes(m)) fail("cadence.time_duplicate", { value: t });
      minutes.push(m);
    }
    minutes.sort((x, y) => x - y);
  }
  if ((c.days ?? []).length === 0) fail("schedule.no_days");
  return { minutes, window: win };
}

/** The next n run times after now (no DST subtleties: the tests run in October before the change). */
export function nextRuns(c: Cadence, now: Date, n: number): string[] {
  const { minutes } = compile(c);
  const out: string[] = [];
  const day = new Date(now);
  day.setHours(0, 0, 0, 0);
  for (let i = 0; i < 60 && out.length < n; i++) {
    const d = new Date(day);
    d.setDate(day.getDate() + i);
    if (!(c.days ?? []).includes(weekdays[d.getDay()]!)) continue;
    for (const m of minutes) {
      const at = new Date(d);
      at.setHours(Math.floor(m / 60), m % 60, 0, 0);
      if (at > now && out.length < n) out.push(at.toISOString());
    }
  }
  return out;
}

/** The send name rule of resolver.SendNameNumbered. */
export function sendName(tpl: string, outName: string, now: Date, lang: "it" | "en", n: number, total: number): string {
  if (tpl === "") return outName;
  let name = renderTokens(tpl, now, lang);
  const ext = /\.[A-Za-z0-9]*[A-Za-z][A-Za-z0-9]*$/.exec(outName)?.[0] ?? "";
  const hasExt = /\.[A-Za-z0-9]*[A-Za-z][A-Za-z0-9]*$/.test(name);
  if (ext && !hasExt) name += ext;
  if (/[<>:"/\\|?*]/.test(name) || /[. ]$/.test(name)) fail("filespec.send_name_invalid", { value: name });
  if (total > 1) {
    const m = /\.[A-Za-z0-9]*[A-Za-z][A-Za-z0-9]*$/.exec(name);
    name = m ? `${name.slice(0, m.index)} (${n})${m[0]}` : `${name} (${n})`;
  }
  return name;
}
