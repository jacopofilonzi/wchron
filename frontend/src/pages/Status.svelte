<script lang="ts">
  // Status and log: channels, runs today, problems in the last 7 days,
  // keep-awake; the last runs (from the log, RunHistory) and the log viewer
  // with a level filter. Opening the page acknowledges the tray's error state.
  import { onMount } from "svelte";
  import { api, ChannelStates, onRunFinished, type AppError, type LogEntry, type RunRecord } from "../lib/api";
  import { day, when } from "../lib/format";
  import { t, tError, type Key } from "../lib/i18n.svelte";
  import { app, applySnapshot } from "../lib/state.svelte";

  onMount(() => {
    void api.ackErrors().then((r) => {
      if (r.ok) applySnapshot(r.value);
    });
  });

  let runs = $state<RunRecord[] | null>(null);
  let runsError = $state<AppError | null>(null);
  let entries = $state<LogEntry[] | null>(null);
  let logError = $state<AppError | null>(null);
  let level = $state("");
  let folderError = $state<AppError | null>(null);

  async function loadRuns() {
    const r = await api.runHistory(500);
    if (r.ok) {
      runs = r.value;
      runsError = null;
    } else runsError = r.error;
  }

  async function loadLog(lv: string) {
    const r = await api.logs(lv, 300);
    if (r.ok) {
      entries = r.value;
      logError = null;
    } else logError = r.error;
  }

  $effect(() => {
    void loadRuns();
    return onRunFinished(() => {
      void loadRuns();
      void loadLog(level);
    });
  });

  $effect(() => {
    void loadLog(level);
  });

  const channels = $derived(app.s?.channel_list ?? []);
  const enabled = $derived(channels.filter((c) => c.enabled));
  const connected = $derived(enabled.filter((c) => c.status.state === ChannelStates.StateConnected).length);

  const today = $derived.by(() => {
    const now = new Date();
    const list = (runs ?? []).filter((r) => r.status !== "skipped" && new Date(r.time).toDateString() === now.toDateString());
    return { runs: list.length, sent: list.reduce((n, r) => n + r.sent, 0) };
  });
  const problems7 = $derived.by(() => {
    const since = Date.now() - 7 * 24 * 3600 * 1000;
    return (runs ?? []).filter((r) => (r.status === "failed" || r.status === "partial") && new Date(r.time).getTime() >= since).length;
  });

  const statusText: Record<string, { key: Key; cls: string }> = {
    ok: { key: "ui.status.run.ok", cls: "text-success" },
    partial: { key: "ui.status.run.partial", cls: "text-warning" },
    failed: { key: "ui.status.run.failed", cls: "text-danger" },
    skipped: { key: "ui.status.run.skipped", cls: "text-muted" },
    dry_run: { key: "ui.schedules.result.dry_run", cls: "text-muted" },
  };

  const levels: { value: string; key: Key }[] = [
    { value: "", key: "ui.status.log.all" },
    { value: "warn", key: "ui.status.log.warnings" },
    { value: "error", key: "ui.status.log.errors" },
  ];

  function clock(iso: string): string {
    const d = new Date(iso);
    const time = d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
    return d.toDateString() === new Date().toDateString() ? time : `${day(d)} ${time}`;
  }

  function attrs(e: LogEntry): string {
    return Object.entries(e.attrs ?? {})
      .map(([k, v]) => `${k}=${typeof v === "string" ? v : JSON.stringify(v)}`)
      .join("  ");
  }

  const levelClass: Record<string, string> = { WARN: "text-warning", ERROR: "text-danger" };

  async function openLogs() {
    const r = await api.openLogsDir();
    folderError = r.ok ? null : r.error;
  }
</script>

<h1 class="page-title">{t("ui.status.title")}</h1>

<div class="grid grid-cols-4 gap-3 text-[13px]">
  <div class="card px-4 py-3.5">
    <div class="text-[12px] text-muted">{t("ui.status.card.channels")}</div>
    <div class="mt-1.5 flex items-center gap-2 font-semibold">
      {#if enabled.length === 0}
        <span class="status-dot bg-warning" aria-hidden="true"></span>{t("ui.channels.none")}
      {:else}
        <span class="status-dot {connected === enabled.length ? 'bg-success' : 'bg-danger'}" aria-hidden="true"></span>
        {t("ui.status.card.channels_value", { connected: String(connected), total: String(enabled.length) })}
      {/if}
    </div>
  </div>
  <div class="card px-4 py-3.5">
    <div class="text-[12px] text-muted">{t("ui.status.card.today")}</div>
    <div class="mt-1.5 font-semibold" data-testid="runs-today">
      {t("ui.status.card.today_value", { runs: String(today.runs), sent: String(today.sent) })}
    </div>
  </div>
  <div class="card px-4 py-3.5">
    <div class="text-[12px] text-muted">{t("ui.status.card.problems")}</div>
    <div class="mt-1.5 font-semibold {problems7 > 0 ? 'text-warning' : ''}" data-testid="problems-7d">
      {problems7 > 0 ? t("ui.status.card.problems_value", { count: String(problems7) }) : t("ui.status.card.problems_none")}
    </div>
  </div>
  <div class="card px-4 py-3.5">
    <div class="text-[12px] text-muted">{t("ui.status.card.keepawake")}</div>
    <div class="mt-1.5 font-semibold">{app.s?.keep_awake ? t("ui.status.on") : t("ui.status.off")}</div>
  </div>
</div>

<section class="card overflow-hidden text-[13px]" aria-labelledby="runs-title">
  <div class="flex items-center justify-between border-b border-divider px-4 py-3">
    <h2 id="runs-title" class="section-title">{t("ui.status.runs")}</h2>
  </div>
  {#if runsError}
    <p role="alert" class="m-0 px-4 py-3 text-danger">{tError(runsError.code, runsError.params)}</p>
  {:else if runs === null}
    <p class="m-0 px-4 py-3 text-muted" aria-busy="true">{t("ui.loading")}</p>
  {:else if runs.length === 0}
    <p class="m-0 px-4 py-6 text-center text-muted">{t("ui.status.empty")}</p>
  {:else}
    <ul class="m-0 max-h-[300px] list-none overflow-y-auto p-0" aria-label={t("ui.status.runs")}>
      {#each runs.slice(0, 50) as r, i (r.time + r.schedule_id + i)}
        {@const st = statusText[r.status]}
        {@const bad = r.status === "failed" || r.status === "partial"}
        <li class="grid grid-cols-[150px_1.3fr_1fr_2fr] gap-3 border-b border-divider px-4 py-2.5 last:border-b-0 {bad ? 'bg-warning-soft' : ''}">
          <span class="tabular-nums">{when(r.time)}</span>
          <span class="truncate">{r.schedule_name}{r.manual ? ` · ${t("ui.status.run.manual")}` : ""}</span>
          <span class={st?.cls ?? "text-muted"}>
            {#if r.status === "skipped"}{t("ui.status.run.skipped")}{:else}{t("ui.status.run.sent", { sent: String(r.sent), planned: String(r.planned) })}{/if}
          </span>
          <span class="line-clamp-2 text-text-soft">
            {r.error ? tError(r.error.code, { schedule: r.schedule_name, ...r.error.params }) : st ? t(st.key) : r.status}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<section class="card flex min-h-[260px] flex-col text-[13px]" aria-labelledby="log-title">
  <div class="flex flex-wrap items-center gap-2 border-b border-divider px-4 py-2.5">
    <h2 id="log-title" class="section-title grow">{t("ui.status.log")}</h2>
    <div class="segmented" role="group" aria-label={t("ui.status.log.filter")}>
      {#each levels as l (l.value)}
        <button type="button" aria-pressed={level === l.value} onclick={() => (level = l.value)}>{t(l.key)}</button>
      {/each}
    </div>
    <button type="button" class="btn h-8" onclick={() => loadLog(level)}>{t("ui.status.log.refresh")}</button>
    <button type="button" class="btn h-8" onclick={openLogs}>{t("ui.status.log.open_folder")}</button>
  </div>
  {#if folderError}<p role="alert" class="m-0 px-4 pt-2 text-danger">{tError(folderError.code, folderError.params)}</p>{/if}
  {#if logError}
    <p role="alert" class="m-0 px-4 py-3 text-danger">{tError(logError.code, logError.params)}</p>
  {:else if entries && entries.length === 0}
    <p class="m-0 px-4 py-6 text-center text-muted">{t("ui.status.log.empty")}</p>
  {:else if entries}
    <ol class="mono m-0 max-h-[420px] list-none overflow-y-auto px-4 py-3 leading-[1.7] text-text-soft select-text" aria-label={t("ui.status.log")} data-testid="log">
      {#each entries as e, i (e.time + i)}
        <li class="wrap-break-word {levelClass[e.level] ?? ''}">
          <span class="tabular-nums">{clock(e.time)}</span>&nbsp;&nbsp;{e.level.padEnd(5)}&nbsp;&nbsp;{e.msg}&nbsp;&nbsp;<span class="text-muted">{attrs(e)}</span>
        </li>
      {/each}
    </ol>
  {/if}
</section>
