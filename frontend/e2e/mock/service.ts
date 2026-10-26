// Fake backend for the end-to-end tests: the same functions as the generated
// internal/app Service bindings, over in-memory data. No Wails, no disk,
// nothing is ever sent. Every Service method must exist here too (a test
// compares the two lists): add it in the same change as the Go method.
import type * as app from "../../bindings/github.com/jacopofilonzi/wchron/internal/app/models";
import type * as config from "../../bindings/github.com/jacopofilonzi/wchron/internal/config/models";
import type * as provider from "../../bindings/github.com/jacopofilonzi/wchron/internal/provider/models";
import { compile, fail, nextRuns, renderTokens, sendName } from "./logic";
import { log, options } from "./options";
import { Events } from "./runtime";

type Lang = "it" | "en";
const clone = <T>(v: T): T => (v === undefined ? v : (JSON.parse(JSON.stringify(v)) as T));
let seq = 0;
const newID = () => `id${++seq}${Math.random().toString(16).slice(2, 8)}`;

// ---------- data ----------

const channelState = options.channels ?? "connected";
const channels: provider.ChannelInfo[] =
  channelState === "none"
    ? []
    : [
        {
          id: "ch1",
          provider: "whatsapp",
          name: "WhatsApp test",
          enabled: true,
          status: {
            state: (channelState === "connected" ? "connected" : "disconnected") as provider.State,
            error: null,
            account: channelState === "connected" ? "***001" : "",
          },
          pairing: false,
        },
      ];

const snapshot: app.Snapshot = {
  routine_active: true,
  keep_awake: false,
  keep_awake_default: false,
  autostart: true,
  theme: (options.theme ?? "light") as config.Theme,
  language: (options.language ?? "it") as app.Snapshot["language"],
  channels: (channels.length === 0 ? "none" : channelState === "connected" ? "connected" : "disconnected") as app.ChannelsState,
  channel_list: channels,
  version: "0.8.0",
  next_run: null,
  sending: false,
  run_error: false,
};
const lang = (): Lang => (snapshot.language === "en" ? "en" : "it");

/** The fake disk: name → size, modification time and (workbooks) sheets. */
const disk: Record<string, { size: number; mtime: string; sheets?: string[] }> = {};
function seedDisk() {
  const d = new Date();
  const stamp = `${String(d.getDate()).padStart(2, "0")}-${String(d.getMonth() + 1).padStart(2, "0")}-${d.getFullYear()}`;
  disk[`C:\\Report\\report_${stamp}.xlsx`] = { size: 24576, mtime: d.toISOString(), sheets: [stamp, "Riepilogo", "Dati"] };
  disk["C:\\Report\\listino.pdf"] = { size: 312000, mtime: d.toISOString() };
  disk["C:\\Report\\vendite_nord.pdf"] = { size: 2048, mtime: d.toISOString() };
  disk["C:\\Report\\vendite_sud.pdf"] = { size: 4096, mtime: new Date(d.getTime() - 3600_000).toISOString() };
}
seedDisk();

const recipient = (id: string, name: string, target: unknown, verified = "yes"): config.Recipient => ({
  id,
  channel_id: "ch1",
  target,
  display_name: name,
  verified: verified as config.Verified,
});
const file = (id: string, path: string, extra: Partial<config.FileSpec> = {}): config.FileSpec => ({
  id,
  path_pattern: path,
  match: "error" as config.MatchMode,
  processor: "passthrough",
  processor_options: {},
  ...extra,
});

let schedules: config.Schedule[] =
  (options.schedules ?? "sample") === "none" || channels.length === 0
    ? []
    : [
        {
          id: "s1",
          name: "Report vendite",
          enabled: true,
          cadence: {
            mode: "interval" as config.CadenceMode,
            interval: { hours: 1, minutes: 0 },
            anchor: "00:00",
            window: { start: "08:00", end: "20:00" },
            times: [],
            days: ["mon", "tue", "wed", "thu", "fri"],
          },
          recipients: [recipient("r1", "Vendite Nord", { kind: "group", jid: "120363001@g.us" })],
          caption: "Report del {date:dddd D MMMM}",
          files: [
            file("f1", "C:\\Report\\report_{date:DD-MM-YYYY}.xlsx", {
              processor: "xlsx-snapshot",
              processor_options: { sheets: ["{date:DD-MM-YYYY}", "Riepilogo"] },
            }),
            file("f2", "C:\\Report\\listino.pdf"),
          ],
        },
        {
          id: "s2",
          name: "Chiusura giornaliera",
          enabled: true,
          cadence: { mode: "fixed" as config.CadenceMode, interval: { hours: 0, minutes: 0 }, anchor: "", window: null, times: ["18:30"], days: ["mon", "tue", "wed", "thu", "fri", "sat", "sun"] },
          recipients: [recipient("r1", "+39 333 ••• 512", { kind: "number", number: "+393331234512" })],
          caption: "",
          files: [file("f1", "C:\\Chiusure\\chiusura_*.pdf")],
        },
      ];

const states: Record<string, { last_result: string; last_error: app.ScheduleSummary["last_error"]; last_run_at: string | null }> = {};
function seedStates() {
  const now = Date.now();
  states.s1 = { last_result: "ok", last_error: null, last_run_at: new Date(now - 30 * 60_000).toISOString() };
  states.s2 = {
    last_result: "failed",
    last_error: { code: "file.not_found" as never, params: { schedule: "Chiusura giornaliera", path: "C:\\Chiusure\\chiusura_*.pdf" } },
    last_run_at: new Date(now - 15 * 3600_000).toISOString(),
  };
}
seedStates();

let errorRecipients: config.Recipient[] = [];

const groups: provider.Target[] = [
  { target: { kind: "group", jid: "120363001@g.us" }, display_name: "Vendite Nord" },
  { target: { kind: "group", jid: "120363002@g.us" }, display_name: "Magazzino" },
  { target: { kind: "group", jid: "120363003@g.us" }, display_name: "Famiglia" },
];

// ---------- helpers ----------

function call<T>(fn: string, args: unknown[], body: () => T): Promise<T> {
  log(fn, ...args);
  return new Promise((resolve, reject) => {
    setTimeout(() => {
      try {
        resolve(clone(body()));
      } catch (e) {
        reject(e);
      }
    }, 5);
  });
}

function changed(id = "") {
  Events.Emit("schedules:changed", { id });
  snapshot.next_run = nextRun();
  emitState();
}

function emitState() {
  Events.Emit("app:state", clone(snapshot));
}

const findSchedule = (id: string) => schedules.find((s) => s.id === id) ?? fail("schedule.not_found", { id });
const absolute = (p: string) => /^[A-Za-z]:\\/.test(p) || p.startsWith("\\\\");
const excelExt = [".xlsx", ".xlsm", ".xltx", ".xltm"];

function globRe(name: string): RegExp {
  return new RegExp("^" + name.replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*/g, ".*").replace(/\?/g, ".") + "$", "i");
}

function render(pattern: string): string {
  if (pattern.trim() === "") fail("filespec.path_required");
  const p = renderTokens(pattern, new Date(), lang());
  if (!absolute(p)) fail("filespec.path_not_absolute", { value: p });
  const i = p.lastIndexOf("\\");
  if (/[*?]/.test(p.slice(0, i))) fail("filespec.glob_in_dir", { value: p });
  return p;
}

function resolve(pattern: string, match: string): { path: string; names: string[] } {
  const p = render(pattern);
  const i = p.lastIndexOf("\\");
  const dir = p.slice(0, i + 1);
  const re = globRe(p.slice(i + 1));
  const found = Object.keys(disk)
    .filter((k) => k.startsWith(dir) && !k.slice(dir.length).includes("\\") && re.test(k.slice(dir.length)))
    .sort();
  if (found.length === 0) fail("file.not_found", { path: p });
  if (found.length > 1 && match === "error") fail("file.multiple_matches", { path: p, count: String(found.length) });
  if (found.length > 1 && match === "newest") {
    found.sort((a, b) => disk[b]!.mtime.localeCompare(disk[a]!.mtime));
    return { path: p, names: [found[0]!] };
  }
  return { path: p, names: found };
}

type Problem = { field: string; code: string; params: Record<string, string> };

function problemOf(field: string, e: unknown): Problem {
  const c = (e as { cause?: { code: string; params: Record<string, string> } }).cause;
  return { field, code: c?.code ?? "internal", params: c?.params ?? {} };
}

const cadenceFields: Record<string, string> = {
  "cadence.interval_invalid": "cadence.interval",
  "cadence.anchor_invalid": "cadence.anchor",
  "cadence.window_invalid": "cadence.window",
  "cadence.window_empty": "cadence.window",
  "cadence.no_times": "cadence.times",
  "cadence.time_invalid": "cadence.times",
  "cadence.time_duplicate": "cadence.times",
  "schedule.no_days": "cadence.days",
};

function recipientProblems(field: string, schedule: string, rs: config.Recipient[]): Problem[] {
  const out: Problem[] = [];
  const seen = new Set<string>();
  for (const r of rs) {
    if (!channels.some((c) => c.id === r.channel_id)) {
      out.push({ field: `${field}.${r.id}`, code: "recipient.channel_missing", params: { schedule, recipient: r.display_name } });
      continue;
    }
    const key = r.channel_id + JSON.stringify(r.target);
    if (seen.has(key)) out.push({ field: `${field}.${r.id}`, code: "recipient.duplicate", params: { recipient: r.display_name } });
    seen.add(key);
  }
  return out;
}

function validate(s: config.Schedule): Problem[] {
  const ps: Problem[] = [];
  const tryAdd = (field: string, fn: () => unknown) => {
    try {
      fn();
    } catch (e) {
      ps.push(problemOf(field, e));
    }
  };
  if (s.name.trim() === "") ps.push({ field: "name", code: "schedule.name_required", params: {} });
  try {
    compile(s.cadence);
  } catch (e) {
    const p = problemOf("cadence", e);
    ps.push({ ...p, field: cadenceFields[p.code] ?? "cadence" });
  }
  const rs = s.recipients ?? [];
  if (rs.length === 0) ps.push({ field: "recipients", code: "recipient.required", params: {} });
  ps.push(...recipientProblems("recipients", s.name, rs));
  tryAdd("caption", () => renderTokens(s.caption, new Date(), lang()));
  const files = s.files ?? [];
  if (files.length === 0) ps.push({ field: "files", code: "schedule.no_files", params: {} });
  for (const f of files) {
    const pre = `files.${f.id}.`;
    let rendered = "";
    tryAdd(pre + "path_pattern", () => (rendered = render(f.path_pattern)));
    if (f.processor === "xlsx-snapshot") {
      const ext = (/\.[^.\\]*$/.exec(rendered)?.[0] ?? "").toLowerCase();
      if (rendered && !/[*?]/.test(ext) && !/[{}]/.test(f.path_pattern.slice(f.path_pattern.lastIndexOf("."))) && !excelExt.includes(ext)) {
        ps.push({ field: pre + "processor", code: "processor.extension_unsupported", params: { ext: ext || rendered } });
      }
    }
    if (f.caption !== null && f.caption !== undefined) tryAdd(pre + "caption", () => renderTokens(f.caption!, new Date(), lang()));
    if (f.send_name) tryAdd(pre + "send_name", () => sendName(f.send_name!, "x", new Date(), lang(), 1, 1));
  }
  return ps;
}

function summary(s: config.Schedule): app.ScheduleSummary {
  const st = states[s.id];
  let next: string | null = null;
  let invalid: app.ScheduleSummary["invalid"] = null;
  try {
    if (s.enabled) next = nextRuns(s.cadence, new Date(), 1)[0] ?? null;
    else compile(s.cadence);
  } catch (e) {
    const p = problemOf("", e);
    invalid = { code: p.code as never, params: p.params };
  }
  return { schedule: s, last_result: st?.last_result ?? "", last_error: st?.last_error ?? null, last_run_at: st?.last_run_at ?? null, next_run: next, invalid };
}

function runRecords(): app.RunRecord[] {
  const now = Date.now();
  const at = (min: number) => new Date(now - min * 60_000).toISOString();
  return [
    { time: at(30), schedule_id: "s1", schedule_name: "Report vendite", manual: false, status: "ok", sent: 2, planned: 2, error: null },
    { time: at(90), schedule_id: "s1", schedule_name: "Report vendite", manual: false, status: "ok", sent: 2, planned: 2, error: null },
    {
      time: at(15 * 60),
      schedule_id: "s2",
      schedule_name: "Chiusura giornaliera",
      manual: false,
      status: "failed",
      sent: 0,
      planned: 1,
      error: { code: "file.not_found" as never, params: { path: "C:\\Chiusure\\chiusura_*.pdf" } },
    },
    {
      time: at(26 * 60),
      schedule_id: "s1",
      schedule_name: "Report vendite",
      manual: false,
      status: "skipped",
      sent: 0,
      planned: 0,
      error: { code: "slot.missed" as never, params: { schedule: "Report vendite", count: "2", from: "06:00", to: "07:00" } },
    },
  ];
}

function logEntries(): { time: string; level: string; msg: string; attrs: Record<string, unknown> }[] {
  const now = Date.now();
  const at = (s: number) => new Date(now - s * 1000).toISOString();
  return [
    { time: at(1790), level: "INFO", msg: "pipeline: run finished", attrs: { schedule: "s1", status: "ok", sent: 2, planned: 2 } },
    { time: at(1795), level: "INFO", msg: "sent document", attrs: { file: "listino.pdf" } },
    { time: at(1800), level: "INFO", msg: "scheduler: run", attrs: { schedule: "s1", name: "Report vendite" } },
    { time: at(54000), level: "WARN", msg: "pipeline: problem", attrs: { schedule: "s2", code: "file.not_found" } },
    { time: at(54100), level: "ERROR", msg: "send failed", attrs: { code: "send.failed" } },
  ];
}

const levelRank: Record<string, number> = { DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3 };

// ---------- Service ----------

function nextRun(): app.Snapshot["next_run"] {
  let best: app.Snapshot["next_run"] = null;
  for (const sc of schedules.filter((x) => x.enabled)) {
    try {
      const at = nextRuns(sc.cadence, new Date(), 1)[0];
      if (at && (!best || at < best.at)) best = { schedule_id: sc.id, schedule_name: sc.name, at };
    } catch {
      // an invalid cadence has no next run
    }
  }
  return best;
}

export const State = () => call("State", [], () => ((snapshot.next_run = nextRun()), snapshot));
export const AckErrors = () => call("AckErrors", [], () => ((snapshot.run_error = false), snapshot));
function setSnap<K extends keyof app.Snapshot>(fn: string, key: K, v: app.Snapshot[K]) {
  return call(fn, [v], () => {
    snapshot[key] = v;
    emitState();
    return snapshot;
  });
}
export const SetRoutineActive = (on: boolean) => setSnap("SetRoutineActive", "routine_active", on);
export const SetKeepAwake = (on: boolean) => setSnap("SetKeepAwake", "keep_awake", on);
export const SetKeepAwakeDefault = (on: boolean) => setSnap("SetKeepAwakeDefault", "keep_awake_default", on);
export const SetAutostart = (on: boolean) => setSnap("SetAutostart", "autostart", on);
export const SetTheme = (t: config.Theme) => setSnap("SetTheme", "theme", t);
export const SetLanguage = (l: app.Snapshot["language"]) => setSnap("SetLanguage", "language", l);

export const Logs = (level: string, limit: number) =>
  call("Logs", [level, limit], () => {
    const min = level ? (levelRank[level.toUpperCase()] ?? 0) : 0;
    return logEntries().filter((e) => (levelRank[e.level] ?? 0) >= min);
  });
export const PreviewTokens = (tpl: string) => call("PreviewTokens", [tpl], () => renderTokens(tpl, new Date(), lang()));
export const PreviewRuns = (c: config.Cadence, n: number) => call("PreviewRuns", [c, n], () => nextRuns(c, new Date(), n || 5));
export const ReadSheets = (pattern: string) =>
  call("ReadSheets", [pattern], () => {
    const r = resolve(pattern, "newest");
    const f = disk[r.names[0]!]!;
    if (!f.sheets) fail("xlsx.not_xlsx", { value: r.names[0]!.split("\\").pop()! });
    return f.sheets;
  });

export const Providers = () =>
  call("Providers", [], (): provider.Info[] => [
    {
      type: "whatsapp",
      capabilities: { files: true, max_file_size: 100 << 20, max_caption_len: 1024, targets: true, pairing: true },
      settings_schema: [],
      target_schema: [],
    },
  ]);
export const ListChannels = () => call("ListChannels", [], () => channels);
export const CreateChannel = (p: string, name: string) => call("CreateChannel", [p, name], () => fail("internal"));
export const RenameChannel = (id: string, name: string) =>
  call("RenameChannel", [id, name], () => {
    const c = channels.find((x) => x.id === id) ?? fail("channel.not_found");
    c.name = name;
    emitState();
    return c;
  });
export const ChannelUsage = (id: string) =>
  call("ChannelUsage", [id], (): provider.Usage => ({
    schedules: schedules.filter((s) => (s.recipients ?? []).some((r) => r.channel_id === id)).map((s) => ({ id: s.id, name: s.name, recipients: 1, disabled: true })),
    error_recipients: errorRecipients.filter((r) => r.channel_id === id).length,
  }));
// Pairing, logout and deleting a channel act for real in the app: the fake refuses them.
export const DeleteChannel = (id: string) => call("DeleteChannel", [id], () => fail("internal"));
export const StartPairing = (id: string) => call("StartPairing", [id], () => fail("internal"));
export const CancelPairing = (id: string) => call("CancelPairing", [id], () => undefined);
export const Logout = (id: string) => call("Logout", [id], () => fail("internal"));

export const ValidateTarget = (channelID: string, target: { kind: string; number?: string }) =>
  call("ValidateTarget", [channelID, target], (): provider.TargetCheck => {
    const ch = channels.find((c) => c.id === channelID) ?? fail("channel.not_found");
    const n = (target.number ?? "").replace(/[\s().-]/g, "").replace(/^00/, "+");
    if (target.kind === "number" && !/^\+\d{8,15}$/.test(n)) fail("wa.number_invalid", { value: target.number ?? "" });
    const norm = { kind: "number", number: n };
    if (ch.status.state !== "connected") {
      return { target: norm, display_name: n, verified: "unknown" as config.Verified, warning: { code: "recipient.check_unavailable" as never, params: { channel: ch.name } } };
    }
    return { target: norm, display_name: n, verified: "yes" as config.Verified, warning: null };
  });
export const ListTargets = (channelID: string) =>
  call("ListTargets", [channelID], () => {
    const ch = channels.find((c) => c.id === channelID) ?? fail("channel.not_found");
    if (ch.status.state !== "connected") fail("channel.not_connected", { channel: ch.name });
    return groups;
  });

// Acts for real in the app: here only recorded.
export const SendNow = (id: string) => call("SendNow", [id], () => void findSchedule(id));

export const ListSchedules = () => call("ListSchedules", [], () => schedules.map(summary));
export const GetSchedule = (id: string) => call("GetSchedule", [id], () => findSchedule(id));
export const NewSchedule = () =>
  call("NewSchedule", [], (): config.Schedule => ({
    id: "",
    name: "",
    enabled: true,
    cadence: { mode: "interval" as config.CadenceMode, interval: { hours: 1, minutes: 0 }, anchor: "00:00", window: null, times: [], days: ["mon", "tue", "wed", "thu", "fri"] },
    recipients: [],
    caption: "",
    files: [],
  }));
export const NewFileSpec = () => call("NewFileSpec", [], () => file(newID(), ""));
export const NewRecipientID = () => call("NewRecipientID", [], () => newID());
export const SaveSchedule = (s: config.Schedule) =>
  call("SaveSchedule", [s], (): app.SaveResult => {
    const sc = clone(s);
    sc.name = sc.name.trim();
    const problems = validate(sc);
    if (problems.length > 0) return { schedule: null, problems };
    if (sc.id === "") {
      sc.id = newID();
      schedules.push(sc);
    } else {
      const i = schedules.findIndex((x) => x.id === sc.id);
      if (i < 0) fail("schedule.not_found", { id: sc.id });
      schedules[i] = sc;
    }
    changed(sc.id);
    return { schedule: sc, problems: [] };
  });
export const DuplicateSchedule = (id: string) =>
  call("DuplicateSchedule", [id], () => {
    const i = schedules.findIndex((x) => x.id === id);
    if (i < 0) fail("schedule.not_found", { id });
    const cp = clone(schedules[i]!);
    cp.id = newID();
    cp.name = lang() === "en" ? `${cp.name} (copy)` : `${cp.name} (copia)`;
    cp.enabled = false;
    schedules.splice(i + 1, 0, cp);
    changed(cp.id);
    return cp;
  });
export const DeleteSchedule = (id: string) =>
  call("DeleteSchedule", [id], () => {
    findSchedule(id);
    schedules = schedules.filter((s) => s.id !== id);
    delete states[id];
    changed(id);
  });
export const SetScheduleEnabled = (id: string, on: boolean) =>
  call("SetScheduleEnabled", [id, on], () => {
    const s = findSchedule(id);
    if (on) {
      const p = validate(s)[0];
      if (p) fail(p.code, p.params);
    }
    s.enabled = on;
    changed(id);
    return s;
  });
export const ProcessorSchemas = () =>
  call("ProcessorSchemas", [], () => [
    { type: "passthrough", extensions: [], options: [] },
    { type: "xlsx-snapshot", extensions: excelExt, options: [{ key: "sheets", kind: "list", required: false, tokens: true }] },
  ]);
export const PreviewFiles = (pattern: string, match: string) =>
  call("PreviewFiles", [pattern, match], (): app.FilePreview => {
    let path = "";
    try {
      path = render(pattern);
      const r = resolve(pattern, match);
      return {
        path,
        files: r.names.map((n) => ({ name: n.split("\\").pop()!, size: disk[n]!.size, mod_time: disk[n]!.mtime })),
        error: null,
      };
    } catch (e) {
      const p = problemOf("", e);
      return { path, files: [], error: { code: p.code as never, params: p.params } };
    }
  });
export const PreviewSendName = (tpl: string, pattern: string, match: string, proc: string) =>
  call("PreviewSendName", [tpl, pattern, match, proc], () => {
    let names: string[];
    try {
      names = resolve(pattern, match).names.map((n) => n.split("\\").pop()!);
    } catch {
      try {
        names = [render(pattern).split("\\").pop()!];
      } catch {
        names = [""];
      }
    }
    const out = (n: string) => (proc === "xlsx-snapshot" ? n.replace(/\.[^.]*$/, "") + ".xlsx" : n);
    return names.map((n, i) => sendName(tpl, out(n), new Date(), lang(), i + 1, names.length));
  });
export const ErrorRecipients = () => call("ErrorRecipients", [], () => errorRecipients);
export const SaveErrorRecipients = (rs: config.Recipient[]) =>
  call("SaveErrorRecipients", [rs], () => {
    const ps = recipientProblems("error_recipients", "", rs);
    if (ps.length === 0) errorRecipients = clone(rs);
    return ps;
  });
export const DataDir = () => call("DataDir", [], () => "C:\\Users\\test\\AppData\\Roaming\\wchron");
export const OpenDataDir = () => call("OpenDataDir", [], () => undefined);
export const OpenLogsDir = () => call("OpenLogsDir", [], () => undefined);
export const LastBackup = () => call("LastBackup", [], () => new Date(Date.now() - 86400_000).toISOString());
export const RunHistory = (limit: number) => call("RunHistory", [limit], () => runRecords().slice(0, limit || 50));
