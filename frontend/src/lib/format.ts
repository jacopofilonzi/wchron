// Display helpers: dates relative to today, sizes, the "when" summary of a
// cadence. Texts come from the catalog (t), dates from Intl in the current
// language.
import { CadenceMode, Langs, type Cadence, type Weekday } from "./api";
import { lang, t, type Key } from "./i18n.svelte";

export const weekdays: Weekday[] = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];

function locale(): string {
  return lang() === Langs.EN ? "en-GB" : "it-IT";
}

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/** "14:00". */
export function time(d: Date): string {
  return d.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit" });
}

/** The day of a date relative to today: "oggi", "domani", "ieri" or "lun 27 ott". */
export function day(d: Date, now = new Date()): string {
  const tomorrow = new Date(now);
  tomorrow.setDate(now.getDate() + 1);
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (sameDay(d, now)) return t("ui.date.today");
  if (sameDay(d, tomorrow)) return t("ui.date.tomorrow");
  if (sameDay(d, yesterday)) return t("ui.date.yesterday");
  return d.toLocaleDateString(locale(), { weekday: "short", day: "numeric", month: "short" });
}

/** "oggi 14:00", "lun 27 ott 09:00". */
export function when(iso: string | Date, now = new Date()): string {
  const d = typeof iso === "string" ? new Date(iso) : iso;
  return `${day(d, now)} ${time(d)}`;
}

/** A full date: "26 ottobre 2026". */
export function longDate(iso: string): string {
  return new Date(iso).toLocaleDateString(locale(), { day: "numeric", month: "long", year: "numeric" });
}

/** "12.3 MB", "512 KB", "800 B" (like the backend's texts). */
export function size(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${Math.floor(n / (1 << 10))} KB`;
  return `${n} B`;
}

export function dayLabel(d: Weekday): string {
  return t(`ui.day.${d}` as Key);
}

/** The active days in words: "tutti i giorni", "lun–ven", "lun, mer, ven". */
export function daysSummary(days: Weekday[]): string {
  const on = weekdays.map((d) => days.includes(d));
  if (on.every(Boolean)) return t("ui.when.every_day");
  if (!on.some(Boolean)) return t("ui.when.no_days");
  const parts: string[] = [];
  for (let i = 0; i < 7; i++) {
    if (!on[i]) continue;
    let j = i;
    while (j + 1 < 7 && on[j + 1]) j++;
    const a = weekdays[i]!;
    const b = weekdays[j]!;
    if (j - i >= 2) parts.push(`${dayLabel(a)}–${dayLabel(b)}`);
    else for (let k = i; k <= j; k++) parts.push(dayLabel(weekdays[k]!));
    i = j;
  }
  return parts.join(", ");
}

/** "1h", "30 min", "1h 30 min". */
export function interval(h: number, m: number): string {
  if (h && m) return t("ui.when.hm", { h: String(h), m: String(m) });
  if (h) return t("ui.when.h", { h: String(h) });
  return t("ui.when.m", { m: String(m) });
}

/** The first line of a cadence's summary: "Ogni 1h · 08:00–20:00", "Alle 18:30". */
export function cadenceSummary(c: Cadence): string {
  if (c.mode === CadenceMode.CadenceFixed) {
    const times = [...(c.times ?? [])].sort();
    return t("ui.when.at", { times: times.join(", ") });
  }
  const every = t("ui.when.every", { interval: interval(c.interval.hours, c.interval.minutes) });
  return c.window ? `${every} · ${c.window.start}–${c.window.end}` : every;
}
