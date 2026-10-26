<script lang="ts">
  // The schedule editor (#/schedules/<id>, #/schedules/new): name, when,
  // recipients, caption, files; next runs, enable, send now, delete. Edits
  // stay in a draft until Save, which Go validates (problems come back under
  // their fields). Leaving with unsaved changes asks first (router guard).
  import { onDestroy } from "svelte";
  import ConfirmDialog from "../components/ConfirmDialog.svelte";
  import Field from "../components/Field.svelte";
  import RecipientList from "../components/RecipientList.svelte";
  import RecipientPicker from "../components/RecipientPicker.svelte";
  import Toggle from "../components/Toggle.svelte";
  import CadenceEditor from "../components/editor/CadenceEditor.svelte";
  import FileSpecCard from "../components/editor/FileSpecCard.svelte";
  import {
    api,
    type AppError,
    type FileSpec,
    type Problem,
    type ProcessorSchema,
    type Recipient,
    type Schedule,
  } from "../lib/api";
  import { day, time } from "../lib/format";
  import { t, tError } from "../lib/i18n.svelte";
  import { go, href, leave, proceed, setLeaveGuard, stay } from "../lib/router.svelte";
  import { app } from "../lib/state.svelte";

  let { id }: { id: string } = $props();

  let draft = $state<Schedule | null>(null);
  let saved = $state("");
  let loadError = $state<AppError | null>(null);
  let schemas = $state<ProcessorSchema[]>([]);
  let problems = $state<Problem[]>([]);
  let saveError = $state<AppError | null>(null);
  let saving = $state(false);
  let notice = $state("");
  let adding = $state(false);

  type Confirm =
    | { kind: "file"; file: FileSpec }
    | { kind: "recipient"; recipient: Recipient }
    | { kind: "delete" }
    | { kind: "send" };
  let confirm = $state<Confirm | null>(null);
  let confirmBusy = $state(false);
  let actionError = $state<AppError | null>(null);

  const isNew = $derived(id === "new");
  const dirty = $derived(draft !== null && JSON.stringify(draft) !== saved);
  const channels = $derived(app.s?.channel_list ?? []);
  const savedSchedule = $derived(saved ? (JSON.parse(saved) as Schedule) : null);

  $effect(() => {
    void (async () => {
      const [s, p] = await Promise.all([isNew ? api.newSchedule() : api.getSchedule(id), api.processorSchemas()]);
      if (p.ok) schemas = p.value;
      if (!s.ok) {
        loadError = s.error;
        return;
      }
      draft = s.value;
      saved = JSON.stringify(s.value);
    })();
  });

  $effect(() => {
    setLeaveGuard(() => dirty);
  });
  onDestroy(() => setLeaveGuard(null));

  function problem(field: string): AppError | null {
    return problems.find((p) => p.field === field)?.error ?? null;
  }
  const recipientErrors = $derived(
    Object.fromEntries(problems.filter((p) => p.field.startsWith("recipients.")).map((p) => [p.field.slice("recipients.".length), p.error])),
  );
  const errorParams = $derived({ schedule: draft?.name || t("ui.editor.this_schedule") });

  // Live previews: next runs (cadence) and caption, debounced.
  let runs = $state<string[]>([]);
  let runsError = $state<AppError | null>(null);
  let captionPreview = $state("");
  let captionError = $state<AppError | null>(null);
  let runSeq = 0;

  $effect(() => {
    if (!draft) return;
    const cadence = JSON.parse(JSON.stringify(draft.cadence));
    const mine = ++runSeq;
    const h = setTimeout(async () => {
      const r = await api.previewRuns(cadence, 5);
      if (mine !== runSeq) return;
      runs = r.ok ? r.value : [];
      runsError = r.ok ? null : r.error;
    }, 200);
    return () => clearTimeout(h);
  });

  $effect(() => {
    if (!draft) return;
    const c = draft.caption;
    const h = setTimeout(async () => {
      const r = await api.previewTokens(c);
      captionPreview = r.ok ? r.value : "";
      captionError = r.ok ? null : r.error;
    }, 250);
    return () => clearTimeout(h);
  });

  async function save() {
    if (!draft) return;
    saving = true;
    notice = "";
    const r = await api.saveSchedule($state.snapshot(draft) as Schedule);
    saving = false;
    if (!r.ok) {
      saveError = r.error;
      return;
    }
    saveError = null;
    problems = r.value.problems;
    if (!r.value.schedule) return;
    draft = r.value.schedule;
    saved = JSON.stringify(r.value.schedule);
    notice = t("ui.editor.saved");
    if (isNew) go(href("schedules", r.value.schedule.id));
  }

  async function addFile() {
    if (!draft) return;
    const r = await api.newFileSpec();
    if (r.ok) draft.files = [...(draft.files ?? []), r.value];
    else actionError = r.error;
  }

  function move(i: number, by: number) {
    if (!draft?.files) return;
    const files = [...draft.files];
    const [f] = files.splice(i, 1);
    if (!f) return;
    files.splice(i + by, 0, f);
    draft.files = files;
  }

  // Removing an item that is already saved asks first (AGENTS.md "Deleting is
  // permanent"); a row added in this session goes at once.
  function askRemoveFile(f: FileSpec) {
    const known = savedSchedule?.files?.some((x) => x.id === f.id) ?? false;
    if (known) confirm = { kind: "file", file: f };
    else if (draft) draft.files = (draft.files ?? []).filter((x) => x.id !== f.id);
  }
  function askRemoveRecipient(r: Recipient) {
    const known = savedSchedule?.recipients?.some((x) => x.id === r.id) ?? false;
    if (known) confirm = { kind: "recipient", recipient: r };
    else if (draft) draft.recipients = (draft.recipients ?? []).filter((x) => x.id !== r.id);
  }

  function addRecipient(r: Recipient) {
    if (!draft) return;
    draft.recipients = [...(draft.recipients ?? []), r];
    adding = false;
  }

  async function confirmed() {
    if (!confirm || !draft) return;
    const c = confirm;
    actionError = null;
    if (c.kind === "file") {
      draft.files = (draft.files ?? []).filter((x) => x.id !== c.file.id);
    } else if (c.kind === "recipient") {
      draft.recipients = (draft.recipients ?? []).filter((x) => x.id !== c.recipient.id);
    } else if (c.kind === "delete") {
      confirmBusy = true;
      const r = await api.deleteSchedule(draft.id);
      confirmBusy = false;
      if (!r.ok) actionError = r.error;
      else {
        setLeaveGuard(null);
        go(href("schedules"));
      }
    } else {
      confirmBusy = true;
      const r = await api.sendNow(draft.id);
      confirmBusy = false;
      if (r.ok) notice = t("ui.editor.send_now.started");
      else actionError = r.error;
    }
    confirm = null;
  }

  function fileLabel(f: FileSpec): string {
    return f.path_pattern.split(/[\\/]/).pop() || f.path_pattern || t("ui.editor.file.unnamed");
  }
</script>

{#if loadError}
  <h1 class="page-title">{t("ui.schedules.title")}</h1>
  <p role="alert" class="m-0 text-danger">{tError(loadError.code, loadError.params)}</p>
  <div><a class="btn no-underline" href={href("schedules")}>{t("ui.editor.back")}</a></div>
{:else if !draft}
  <div class="card px-6 py-10 text-center text-[13px] text-muted" aria-busy="true">{t("ui.loading")}</div>
{:else}
  <header class="flex flex-wrap items-center gap-2.5">
    <div class="min-w-0 grow">
      <nav class="text-[13px] text-muted" aria-label={t("ui.editor.breadcrumb")}>
        <a class="text-muted no-underline hover:underline" href={href("schedules")}>{t("ui.schedules.title")}</a>
        <span aria-hidden="true"> / </span>
        <span>{isNew ? t("ui.editor.new_title") : savedSchedule?.name}</span>
      </nav>
      <h1 class="page-title mt-1 truncate">{draft.name || (isNew ? t("ui.editor.new_title") : savedSchedule?.name)}</h1>
    </div>
    {#if dirty}<span class="text-[12px] text-warning" role="status">{t("ui.editor.unsaved")}</span>{:else if notice}<span
        class="text-[12px] text-success"
        role="status">{notice}</span
      >{/if}
    <a class="btn h-9 no-underline" href={href("schedules")}>{isNew || dirty ? t("ui.dialog.cancel") : t("ui.editor.back")}</a>
    <button type="button" class="btn btn-primary h-9" disabled={saving || (!dirty && !isNew)} onclick={save}>{t("ui.editor.save")}</button>
  </header>

  {#if problems.length > 0}
    <p role="alert" class="card m-0 border-danger px-4 py-3 text-[13px] text-danger">
      {t("ui.editor.problems", { count: String(problems.length) })}
    </p>
  {/if}
  {#if saveError}<p role="alert" class="m-0 text-danger">{tError(saveError.code, saveError.params)}</p>{/if}
  {#if actionError}<p role="alert" class="m-0 text-danger">{tError(actionError.code, { ...errorParams, name: draft.name, ...actionError.params })}</p>{/if}

  <div class="flex items-start gap-4">
    <div class="flex min-w-0 grow flex-col gap-4">
      <section class="section-card" aria-label={t("ui.editor.general")}>
        <Field id="sched-name" label={t("ui.editor.name")} error={problem("name")}>
          <input
            id="sched-name"
            class="input"
            maxlength="120"
            bind:value={draft.name}
            placeholder={t("ui.editor.name_placeholder")}
            aria-describedby={problem("name") ? "sched-name-error" : undefined}
          />
        </Field>
      </section>

      <CadenceEditor bind:cadence={draft.cadence} {problem} live={runsError} />

      <section class="section-card" aria-labelledby="who-title">
        <div class="flex items-center justify-between">
          <h2 id="who-title" class="section-title">{t("ui.editor.who")}</h2>
          {#if !adding}<button type="button" class="btn" onclick={() => (adding = true)}>{t("ui.recipients.add")}</button>{/if}
        </div>
        <RecipientList
          recipients={draft.recipients ?? []}
          {channels}
          label={t("ui.editor.who")}
          errors={recipientErrors}
          {errorParams}
          onremove={askRemoveRecipient}
        />
        {#if (draft.recipients ?? []).length === 0 && !adding}<p class="m-0 text-muted">{t("ui.editor.who.empty")}</p>{/if}
        {#if adding}<RecipientPicker {channels} onadd={addRecipient} oncancel={() => (adding = false)} />{/if}
        {#if problem("recipients")}<p role="alert" class="m-0 text-danger">{tError(problem("recipients")!.code, problem("recipients")!.params)}</p>{/if}
      </section>

      <section class="section-card" aria-labelledby="caption-title">
        <h2 id="caption-title" class="section-title">{t("ui.editor.caption")}</h2>
        <Field id="sched-caption" label={t("ui.editor.caption.label")} hint={t("ui.editor.caption.hint")} error={problem("caption") ?? captionError}>
          <input id="sched-caption" class="input mono" bind:value={draft.caption} placeholder={t("ui.editor.caption.placeholder")} />
        </Field>
        {#if draft.caption !== "" && !captionError}
          <p class="m-0 text-muted" data-testid="caption-preview">{t("ui.editor.preview_now")} <span class="text-text">{captionPreview}</span></p>
        {/if}
      </section>

      <section class="section-card" aria-labelledby="files-title">
        <div class="flex items-center justify-between">
          <h2 id="files-title" class="section-title">{t("ui.editor.files")}</h2>
          <button type="button" class="btn" onclick={addFile}>{t("ui.editor.files.add")}</button>
        </div>
        {#if (draft.files ?? []).length === 0}
          <p class="m-0 text-muted">{t("ui.editor.files.empty")}</p>
        {:else}
          <ol class="m-0 flex list-none flex-col gap-3 p-0" aria-label={t("ui.editor.files")}>
            {#each draft.files ?? [] as f, i (f.id)}
              <FileSpecCard
                bind:file={draft.files![i]!}
                index={i}
                count={draft.files?.length ?? 0}
                {schemas}
                scheduleName={draft.name}
                {problem}
                onup={() => move(i, -1)}
                ondown={() => move(i, 1)}
                onremove={() => askRemoveFile(f)}
              />
            {/each}
          </ol>
        {/if}
        {#if problem("files")}<p role="alert" class="m-0 text-danger">{tError(problem("files")!.code, problem("files")!.params)}</p>{/if}
      </section>
    </div>

    <aside class="flex w-[250px] shrink-0 flex-col gap-4">
      <section class="section-card gap-2.5" aria-labelledby="next-title">
        <h2 id="next-title" class="section-title">{t("ui.editor.next_runs")}</h2>
        {#if runsError}
          <p class="m-0 text-danger">{tError(runsError.code, runsError.params)}</p>
        {:else if runs.length === 0}
          <p class="m-0 text-muted">{t("ui.editor.next_runs.none")}</p>
        {:else}
          <ol class="m-0 list-none p-0" data-testid="next-runs">
            {#each runs as r, i (r)}
              {@const d = new Date(r)}
              <li class="flex justify-between border-b border-divider py-1.5 last:border-b-0">
                <span>{day(d)}</span><span class="tabular-nums {i === 0 ? 'font-semibold' : ''}">{time(d)}</span>
              </li>
            {/each}
          </ol>
        {/if}
        <p class="m-0 text-[12px] leading-normal text-muted">{t("ui.editor.next_runs.note")}</p>
      </section>

      <section class="section-card gap-3" aria-label={t("ui.editor.actions")}>
        <div class="flex items-center justify-between">
          <span class="font-medium" id="enabled-label">{t("ui.editor.enabled")}</span>
          <Toggle pressed={draft.enabled} label={t("ui.editor.enabled")} onchange={(on) => draft && (draft.enabled = on)} />
        </div>
        {#if !isNew}
          <button type="button" class="btn justify-center" disabled={dirty} onclick={() => (confirm = { kind: "send" })}>{t("ui.editor.send_now")}</button>
          {#if dirty}<p class="m-0 text-[12px] text-muted">{t("ui.editor.send_now.save_first")}</p>{/if}
          <button type="button" class="btn btn-danger-outline justify-center" onclick={() => (confirm = { kind: "delete" })}>{t("ui.editor.delete")}</button>
        {/if}
      </section>
    </aside>
  </div>
{/if}

{#if leave.pending !== null}
  <ConfirmDialog title={t("ui.editor.leave.title")} confirmLabel={t("ui.editor.leave.discard")} onconfirm={proceed} oncancel={stay}>
    <p class="m-0 text-sm">{t("ui.editor.leave.confirm")}</p>
  </ConfirmDialog>
{/if}

{#if confirm && draft}
  {@const c = confirm}
  {#if c.kind === "file"}
    <ConfirmDialog title={t("ui.editor.file.remove.title")} confirmLabel={t("ui.editor.file.remove")} onconfirm={confirmed} oncancel={() => (confirm = null)}>
      <p class="m-0 text-sm">{t("ui.editor.file.remove.confirm", { name: fileLabel(c.file) })}</p>
      <p class="m-0 text-text-soft">{t("ui.editor.remove.consequences")}</p>
    </ConfirmDialog>
  {:else if c.kind === "recipient"}
    <ConfirmDialog title={t("ui.recipients.remove.title")} confirmLabel={t("ui.recipients.remove")} onconfirm={confirmed} oncancel={() => (confirm = null)}>
      <p class="m-0 text-sm">{t("ui.recipients.remove.confirm", { name: c.recipient.display_name })}</p>
      <p class="m-0 text-text-soft">{t("ui.editor.remove.consequences")}</p>
    </ConfirmDialog>
  {:else if c.kind === "delete"}
    <ConfirmDialog title={t("ui.schedules.delete.title")} confirmLabel={t("ui.schedules.delete")} busy={confirmBusy} onconfirm={confirmed} oncancel={() => (confirm = null)}>
      <p class="m-0 text-sm">{t("ui.schedules.delete.confirm", { name: savedSchedule?.name ?? draft.name })}</p>
      <p class="m-0 text-text-soft">{t("ui.schedules.delete.consequences")}</p>
    </ConfirmDialog>
  {:else}
    <ConfirmDialog title={t("ui.editor.send_now.title")} confirmLabel={t("ui.editor.send_now")} busy={confirmBusy} onconfirm={confirmed} oncancel={() => (confirm = null)}>
      <p class="m-0 text-sm">
        {t("ui.editor.send_now.confirm", { name: draft.name, recipients: String(draft.recipients?.length ?? 0), files: String(draft.files?.length ?? 0) })}
      </p>
      <p class="m-0 text-text-soft">{t("ui.editor.send_now.consequences")}</p>
    </ConfirmDialog>
  {/if}
{/if}
