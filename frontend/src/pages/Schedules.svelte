<script lang="ts">
  // Schedules: the list (enable toggle, when, recipients, next run, last
  // result; open, duplicate, delete) or, under #/schedules/<id>, the editor.
  import ConfirmDialog from "../components/ConfirmDialog.svelte";
  import Icon from "../components/Icon.svelte";
  import Toggle from "../components/Toggle.svelte";
  import ScheduleEditor from "./ScheduleEditor.svelte";
  import { api, onRunFinished, onSchedulesChanged, type AppError, type ScheduleSummary } from "../lib/api";
  import { cadenceSummary, daysSummary, when } from "../lib/format";
  import { t, tError, type Key } from "../lib/i18n.svelte";
  import { href, route } from "../lib/router.svelte";
  import { app } from "../lib/state.svelte";
  import { providerUIs } from "../providers";

  let list = $state<ScheduleSummary[] | null>(null);
  let loadError = $state<AppError | null>(null);
  let rowErrors = $state<Record<string, AppError>>({});
  let confirmDelete = $state<ScheduleSummary | null>(null);
  let busy = $state(false);

  const channels = $derived(app.s?.channel_list ?? []);

  async function load() {
    const r = await api.listSchedules();
    if (r.ok) {
      list = r.value;
      loadError = null;
    } else loadError = r.error;
  }

  $effect(() => {
    if (route.param !== "") return;
    void load();
    const offs = [onSchedulesChanged(load), onRunFinished(load)];
    return () => offs.forEach((off) => off());
  });

  function setError(id: string, e: AppError | null) {
    if (e) rowErrors[id] = e;
    else delete rowErrors[id];
  }

  async function toggle(s: ScheduleSummary, on: boolean) {
    const r = await api.setScheduleEnabled(s.schedule.id, on);
    setError(s.schedule.id, r.ok ? null : r.error);
    if (r.ok) await load();
  }

  async function duplicate(s: ScheduleSummary) {
    const r = await api.duplicateSchedule(s.schedule.id);
    setError(s.schedule.id, r.ok ? null : r.error);
    if (r.ok) await load();
  }

  async function remove() {
    if (!confirmDelete) return;
    busy = true;
    const id = confirmDelete.schedule.id;
    const r = await api.deleteSchedule(id);
    busy = false;
    setError(id, r.ok ? null : r.error);
    confirmDelete = null;
    if (r.ok) await load();
  }

  function recipientsSummary(s: ScheduleSummary): { main: string; sub: string } {
    const rs = s.schedule.recipients ?? [];
    if (rs.length === 0) return { main: t("ui.schedules.no_recipients"), sub: "" };
    const first = rs[0]!;
    if (rs.length > 1) return { main: first.display_name, sub: t("ui.schedules.more_recipients", { count: String(rs.length - 1) }) };
    const ch = channels.find((c) => c.id === first.channel_id);
    const kind = ch ? providerUIs[ch.provider]?.targetKind(first.target) : null;
    return { main: first.display_name, sub: kind ? t(kind) : (ch?.name ?? "") };
  }

  function filesSummary(s: ScheduleSummary): string {
    const files = s.schedule.files ?? [];
    const parts = [t("ui.schedules.files", { count: String(files.length) })];
    const kinds = [...new Set(files.map((f) => f.processor).filter((p) => p !== "passthrough"))];
    for (const k of kinds) parts.push(t(`ui.processor.${k}.name` as Key));
    if (!s.schedule.enabled) parts.push(t("ui.schedules.disabled"));
    return parts.join(" · ");
  }

  const results: Record<string, { key: Key; dot: string; text: string }> = {
    ok: { key: "ui.schedules.result.ok", dot: "bg-success", text: "text-success" },
    dry_run: { key: "ui.schedules.result.dry_run", dot: "bg-idle", text: "text-muted" },
    partial: { key: "ui.schedules.result.partial", dot: "bg-warning", text: "text-warning" },
    failed: { key: "ui.schedules.result.failed", dot: "bg-danger", text: "text-danger" },
    skipped: { key: "ui.schedules.result.skipped", dot: "bg-idle", text: "text-muted" },
  };

  const next = $derived(app.s?.next_run ?? null);
</script>

{#if route.param !== ""}
  {#key route.param}
    <ScheduleEditor id={route.param} />
  {/key}
{:else}
  <header class="flex items-center justify-between gap-4">
    <div>
      <h1 class="page-title">{t("ui.schedules.title")}</h1>
      {#if list && list.length > 0}
        <p class="m-0 mt-1 text-[13px] text-muted">
          {list.length === 1 ? t("ui.schedules.count.one") : t("ui.schedules.count", { count: String(list.length) })}{#if next}{" · "}{t("ui.schedules.next_send", { when: when(next.at) })}{/if}
        </p>
      {/if}
    </div>
    {#if channels.length > 0}
      <a class="btn btn-primary h-9 no-underline" href={href("schedules", "new")}>{t("ui.schedules.new")}</a>
    {/if}
  </header>

  {#if loadError}<p role="alert" class="m-0 text-danger">{tError(loadError.code, loadError.params)}</p>{/if}

  {#if list === null}
    <div class="card px-6 py-10 text-center text-[13px] text-muted" aria-busy="true">{t("ui.loading")}</div>
  {:else if list.length === 0 && channels.length === 0}
    <div class="card flex flex-col items-center gap-3 px-6 py-10 text-center text-[13px]">
      <p class="m-0 text-sm font-medium">{t("ui.schedules.empty_channels.title")}</p>
      <p class="m-0 max-w-[460px] text-muted">{t("ui.schedules.empty_channels.text")}</p>
      <a class="btn btn-primary no-underline" href={href("channels")}>{t("ui.schedules.empty_channels.action")}</a>
    </div>
  {:else if list.length === 0}
    <div class="card flex flex-col items-center gap-3 px-6 py-10 text-center text-[13px]">
      <p class="m-0 text-sm font-medium">{t("ui.schedules.empty.title")}</p>
      <p class="m-0 max-w-[460px] text-muted">{t("ui.schedules.empty.text")}</p>
      <a class="btn btn-primary no-underline" href={href("schedules", "new")}>{t("ui.schedules.new")}</a>
    </div>
  {:else}
    <div class="card overflow-hidden text-[13px]" role="table" aria-label={t("ui.schedules.list")}>
      <div
        role="row"
        class="grid grid-cols-[44px_minmax(0,1.5fr)_minmax(0,1.3fr)_minmax(0,1.1fr)_minmax(0,0.9fr)_minmax(0,1.5fr)_64px] gap-3 border-b border-divider px-4 py-2.5 text-[12px] text-muted"
      >
        <span role="columnheader"><span class="sr-only">{t("ui.schedules.col.enabled")}</span></span>
        <span role="columnheader">{t("ui.schedules.col.name")}</span>
        <span role="columnheader">{t("ui.schedules.col.when")}</span>
        <span role="columnheader">{t("ui.schedules.col.recipients")}</span>
        <span role="columnheader">{t("ui.schedules.col.next")}</span>
        <span role="columnheader">{t("ui.schedules.col.last")}</span>
        <span role="columnheader"><span class="sr-only">{t("ui.schedules.col.actions")}</span></span>
      </div>
      {#each list as s (s.schedule.id)}
        {@const sc = s.schedule}
        {@const rs = recipientsSummary(s)}
        {@const res = results[s.last_result]}
        <div
          role="row"
          class="grid grid-cols-[44px_minmax(0,1.5fr)_minmax(0,1.3fr)_minmax(0,1.1fr)_minmax(0,0.9fr)_minmax(0,1.5fr)_64px] items-center gap-3 border-b border-divider px-4 py-3.5 last:border-b-0"
        >
          <span role="cell">
            <Toggle
              pressed={sc.enabled}
              label={t("ui.schedules.enable_named", { name: sc.name })}
              onchange={(on) => toggle(s, on)}
            />
          </span>
          <div role="cell" class="min-w-0">
            <a aria-label={t("ui.schedules.open_named", { name: sc.name })}
              class="block truncate font-medium text-text no-underline hover:underline {sc.enabled ? '' : 'text-muted'}" href={href("schedules", sc.id)}>{sc.name}</a>
            <div class="mt-0.5 truncate text-[12px] text-muted">{filesSummary(s)}</div>
          </div>
          <div role="cell" class="min-w-0 {sc.enabled ? 'text-text-soft' : 'text-muted'}">
            <div class="truncate" title={cadenceSummary(sc.cadence)}>{cadenceSummary(sc.cadence)}</div>
            <div class="truncate text-[12px] text-muted">{daysSummary(sc.cadence.days ?? [])}</div>
          </div>
          <div role="cell" class="min-w-0">
            <div class="truncate">{rs.main}</div>
            {#if rs.sub}<div class="truncate text-[12px] text-muted">{rs.sub}</div>{/if}
          </div>
          <div role="cell" class="tabular-nums">
            {#if s.invalid}
              <span class="text-danger" title={tError(s.invalid.code, { ...s.invalid.params })}>{t("ui.schedules.invalid")}</span>
            {:else if s.next_run}
              <span class="font-medium">{when(s.next_run)}</span>
            {:else}
              <span class="text-muted">—</span>
            {/if}
          </div>
          <div role="cell" class="min-w-0">
            {#if res}
              <div class="flex items-center gap-1.5 {res.text}">
                <span class="status-dot size-1.5 {res.dot}" aria-hidden="true"></span>
                <span class="truncate">{t(res.key)}{s.last_run_at ? ` · ${when(s.last_run_at)}` : ""}</span>
              </div>
              {#if s.last_error}
                {@const text = tError(s.last_error.code, { schedule: sc.name, ...s.last_error.params })}
                <div class="mt-0.5 line-clamp-2 text-[12px] text-muted" title={text}>{text}</div>
              {/if}
            {:else}
              <span class="text-muted">{t("ui.schedules.result.never")}</span>
            {/if}
          </div>
          <div role="cell" class="flex justify-end gap-0.5">
            <button
              type="button"
              class="icon-btn"
              aria-label={t("ui.schedules.duplicate_named", { name: sc.name })}
              title={t("ui.schedules.duplicate")}
              onclick={() => duplicate(s)}><Icon name="copy" /></button
            >
            <button
              type="button"
              class="icon-btn hover:text-danger"
              aria-label={t("ui.schedules.delete_named", { name: sc.name })}
              title={t("ui.schedules.delete")}
              onclick={() => (confirmDelete = s)}><Icon name="trash" /></button
            >
          </div>
          {#if rowErrors[sc.id]}
            <p role="alert" class="col-span-full m-0 text-danger">{tError(rowErrors[sc.id]!.code, { schedule: sc.name, ...rowErrors[sc.id]!.params })}</p>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
{/if}

{#if confirmDelete}
  <ConfirmDialog
    title={t("ui.schedules.delete.title")}
    confirmLabel={t("ui.schedules.delete")}
    {busy}
    onconfirm={remove}
    oncancel={() => (confirmDelete = null)}
  >
    <p class="m-0 text-sm">{t("ui.schedules.delete.confirm", { name: confirmDelete.schedule.name })}</p>
    <p class="m-0 text-text-soft">{t("ui.schedules.delete.consequences")}</p>
  </ConfirmDialog>
{/if}
