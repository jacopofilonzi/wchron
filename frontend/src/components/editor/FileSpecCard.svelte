<script lang="ts">
  // One file of a schedule: path pattern with what it resolves to now, match
  // mode, processing (from ProcessorSchemas, filtered by the extension) and
  // its options, own caption, name to send with its live preview (computed
  // in Go). Reorder and remove are the page's.
  import ChipsInput from "../ChipsInput.svelte";
  import Icon from "../Icon.svelte";
  import SegmentedControl from "../SegmentedControl.svelte";
  import {
    api,
    MatchMode,
    ProcessorFieldKind,
    type AppError,
    type FilePreview,
    type FileSpec,
    type ProcessorSchema,
  } from "../../lib/api";
  import { size } from "../../lib/format";
  import { has, t, tError, type Key } from "../../lib/i18n.svelte";

  let {
    file = $bindable(),
    index,
    count,
    schemas,
    scheduleName,
    problem,
    onup,
    ondown,
    onremove,
  }: {
    file: FileSpec;
    index: number;
    count: number;
    schemas: ProcessorSchema[];
    scheduleName: string;
    problem: (field: string) => AppError | null;
    onup: () => void;
    ondown: () => void;
    onremove: () => void;
  } = $props();

  const uid = $derived(`file-${file.id}`);
  const p = (field: string) => problem(`files.${file.id}.${field}`);
  // Error texts written for run time name the schedule.
  const params = $derived({ schedule: scheduleName || t("ui.editor.this_schedule") });

  // The extension the pattern fixes ("" when a wildcard or a token is in it).
  const ext = $derived.by(() => {
    const name = file.path_pattern.split(/[\\/]/).pop() ?? "";
    const dot = name.lastIndexOf(".");
    if (dot <= 0) return "";
    const e = name.slice(dot).toLowerCase();
    return /[*?{}]/.test(e) ? "" : e;
  });
  const compatible = $derived(
    schemas.filter((s) => (s.extensions ?? []).length === 0 || ext === "" || (s.extensions ?? []).includes(ext) || s.type === file.processor),
  );
  const schema = $derived(schemas.find((s) => s.type === file.processor));

  // Live previews, debounced; a counter drops answers that arrive late.
  let preview = $state<FilePreview | null>(null);
  let names = $state<string[]>([]);
  let nameError = $state<AppError | null>(null);
  let captionPreview = $state("");
  let captionError = $state<AppError | null>(null);
  let seq = 0;

  $effect(() => {
    const pattern = file.path_pattern;
    const match = file.match;
    const tpl = file.send_name ?? "";
    const proc = file.processor;
    const mine = ++seq;
    const h = setTimeout(async () => {
      if (pattern.trim() === "") {
        preview = null;
        names = [];
        return;
      }
      const [r, n] = await Promise.all([api.previewFiles(pattern, match), api.previewSendName(tpl, pattern, match, proc)]);
      if (mine !== seq) return;
      preview = r.ok ? r.value : null;
      if (n.ok) {
        names = n.value;
        nameError = null;
      } else {
        names = [];
        nameError = n.error;
      }
    }, 250);
    return () => clearTimeout(h);
  });

  $effect(() => {
    const c = file.caption;
    if (c === null || c === undefined) return;
    const h = setTimeout(async () => {
      const r = await api.previewTokens(c);
      captionPreview = r.ok ? r.value : "";
      captionError = r.ok ? null : r.error;
    }, 250);
    return () => clearTimeout(h);
  });

  // Options: processor_options is a plain object of the schema's keys.
  function opts(): Record<string, unknown> {
    const o = file.processor_options;
    return o && typeof o === "object" ? (o as Record<string, unknown>) : {};
  }
  function listOpt(key: string): string[] {
    const v = opts()[key];
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
  }
  function setOpt(key: string, v: unknown) {
    file.processor_options = { ...opts(), [key]: v };
  }

  // Token previews of the list values (sheet names with dates).
  let valuePreviews = $state<Record<string, string>>({});
  $effect(() => {
    const values = (schema?.options ?? []).filter((f) => f.tokens && f.kind === ProcessorFieldKind.FieldList).flatMap((f) => listOpt(f.key));
    const h = setTimeout(async () => {
      const out: Record<string, string> = {};
      for (const v of values.filter((x) => x.includes("{"))) {
        const r = await api.previewTokens(v);
        out[v] = r.ok ? r.value : t("ui.editor.token_error");
      }
      valuePreviews = out;
    }, 250);
    return () => clearTimeout(h);
  });

  // "Read the sheets from the file": the sheet names to pick from.
  let sheets = $state<string[] | null>(null);
  let sheetsError = $state<AppError | null>(null);
  let sheetsBusy = $state(false);
  async function readSheets() {
    sheetsBusy = true;
    const r = await api.readSheets(file.path_pattern);
    sheetsBusy = false;
    sheets = r.ok ? r.value : null;
    sheetsError = r.ok ? null : r.error;
  }
  // The value that selects a sheet: its name, or a token that renders to it now.
  function selectedSheet(key: string, name: string): string | null {
    return listOpt(key).find((v) => v === name || valuePreviews[v] === name) ?? null;
  }
  function toggleSheet(key: string, name: string) {
    const cur = listOpt(key);
    const sel = selectedSheet(key, name);
    setOpt(key, sel !== null ? cur.filter((x) => x !== sel) : [...cur, name]);
  }

  const matchHints: Record<string, Key> = {
    [MatchMode.MatchError]: "ui.editor.file.match.error_hint",
    [MatchMode.MatchNewest]: "ui.editor.file.match.newest_hint",
    [MatchMode.MatchAll]: "ui.editor.file.match.all_hint",
  };
  const hasGlob = $derived(/[*?]/.test(file.path_pattern.split(/[\\/]/).pop() ?? ""));

  function procLabel(type: string): string {
    const k = `ui.processor.${type}.name`;
    return has(k) ? t(k) : type;
  }
</script>

{#snippet err(field: string)}
  {@const e = p(field)}
  {#if e}<p id="{uid}-{field}-error" role="alert" class="m-0 text-danger">{tError(e.code, { ...params, ...e.params })}</p>{/if}
{/snippet}

<li class="flex flex-col gap-3 rounded-[10px] border border-border p-3.5" aria-label={t("ui.editor.file.label", { n: String(index + 1) })}>
  <div class="flex items-center gap-2">
    <span class="w-5 text-right text-muted tabular-nums" aria-hidden="true">{index + 1}.</span>
    <label for="{uid}-path" class="sr-only">{t("ui.editor.file.path")}</label>
    <input
      id="{uid}-path"
      class="input mono grow"
      placeholder={t("ui.editor.file.path_placeholder")}
      spellcheck="false"
      aria-invalid={p("path_pattern") !== null}
      bind:value={file.path_pattern}
    />
    <button type="button" class="icon-btn" disabled={index === 0} aria-label={t("ui.editor.file.up")} onclick={onup}><Icon name="up" /></button>
    <button type="button" class="icon-btn" disabled={index === count - 1} aria-label={t("ui.editor.file.down")} onclick={ondown}><Icon name="down" /></button>
    <button type="button" class="icon-btn" aria-label={t("ui.editor.file.remove")} onclick={onremove}><Icon name="close" /></button>
  </div>
  <div class="flex flex-col gap-3 pl-7">
    {@render err("path_pattern")}
    {#if preview}
      {#if preview.error}
        {@const fileErr = preview.error.code.startsWith("file.")}
        <p class="m-0 {fileErr ? 'text-warning' : 'text-danger'}" data-testid="file-preview">
          {tError(preview.error.code, { ...params, ...(preview.error.params ?? {}) })}
        </p>
      {:else}
        <p class="m-0 text-success" data-testid="file-preview">
          {t("ui.editor.file.now", {
            files: (preview.files ?? []).map((f) => `${f.name} (${size(f.size)})`).join(", "),
          })}
        </p>
      {/if}
    {:else if file.path_pattern.trim() === ""}
      <p class="m-0 text-[12px] text-muted">{t("ui.editor.file.path_hint")}</p>
    {/if}

    {#if hasGlob}
      <div class="flex flex-col gap-1.5">
        <span class="text-muted" id="{uid}-match-label">{t("ui.editor.file.match")}</span>
        <SegmentedControl
          label={t("ui.editor.file.match")}
          options={[
            { value: MatchMode.MatchError, label: t("ui.editor.file.match.error") },
            { value: MatchMode.MatchNewest, label: t("ui.editor.file.match.newest") },
            { value: MatchMode.MatchAll, label: t("ui.editor.file.match.all") },
          ]}
          value={file.match}
          onchange={(m) => (file.match = m)}
        />
        <p class="m-0 text-[12px] text-muted">{t(matchHints[file.match] ?? "ui.editor.file.match.error_hint")}</p>
      </div>
    {/if}

    <div class="flex flex-col gap-1.5">
      <label for="{uid}-proc" class="text-muted">{t("ui.editor.file.processor")}</label>
      <select
        id="{uid}-proc"
        class="field w-[260px]"
        value={file.processor}
        onchange={(e) => {
          file.processor = e.currentTarget.value;
          file.processor_options = {};
          sheets = null;
        }}
      >
        {#each compatible as s (s.type)}<option value={s.type}>{procLabel(s.type)}</option>{/each}
      </select>
      {#if has(`ui.processor.${file.processor}.description`)}
        <p class="m-0 text-[12px] text-muted">{t(`ui.processor.${file.processor}.description` as Key)}</p>
      {/if}
      {@render err("processor")}
    </div>

    {#each schema?.options ?? [] as f (f.key)}
      {@const labelKey = `ui.processor.${file.processor}.${f.key}`}
      {@const hintKey = `${labelKey}.hint`}
      <div class="flex flex-col gap-1.5">
        <label for="{uid}-opt-{f.key}" class="text-muted">{has(labelKey) ? t(labelKey) : f.key}</label>
        {#if f.kind === ProcessorFieldKind.FieldList}
          <ChipsInput
            id="{uid}-opt-{f.key}"
            label={has(labelKey) ? t(labelKey) : f.key}
            mono={f.tokens}
            preview={valuePreviews}
            bind:values={() => listOpt(f.key), (v) => setOpt(f.key, v)}
          />
          {#if f.key === "sheets"}
            <div>
              <button type="button" class="link-btn" disabled={sheetsBusy || file.path_pattern.trim() === ""} onclick={readSheets}>
                {t("ui.processor.xlsx-snapshot.read_sheets")}
              </button>
            </div>
            {#if sheetsError}
              <p role="alert" class="m-0 text-danger">{tError(sheetsError.code, { ...params, ...sheetsError.params })}</p>
            {:else if sheets}
              <div class="flex flex-wrap gap-1.5" role="group" aria-label={t("ui.editor.file.sheets_found")}>
                {#each sheets as name (name)}
                  <button type="button" class="chip-toggle" aria-pressed={selectedSheet(f.key, name) !== null} onclick={() => toggleSheet(f.key, name)}>{name}</button>
                {/each}
              </div>
            {/if}
          {/if}
        {:else}
          <input
            id="{uid}-opt-{f.key}"
            class="input {f.tokens ? 'mono' : ''}"
            value={typeof opts()[f.key] === "string" ? (opts()[f.key] as string) : ""}
            oninput={(e) => setOpt(f.key, e.currentTarget.value)}
          />
        {/if}
        {#if has(hintKey)}<p class="m-0 text-[12px] text-muted">{t(hintKey)}</p>{/if}
      </div>
    {/each}
    {@render err("processor_options")}

    <div class="flex flex-col gap-1.5">
      <label class="flex items-center gap-2">
        <input
          type="checkbox"
          checked={file.caption !== null && file.caption !== undefined}
          onchange={(e) => (file.caption = e.currentTarget.checked ? "" : null)}
        />
        {t("ui.editor.file.own_caption")}
      </label>
      {#if file.caption !== null && file.caption !== undefined}
        <label for="{uid}-caption" class="sr-only">{t("ui.editor.file.caption")}</label>
        <input id="{uid}-caption" class="input mono" bind:value={file.caption} placeholder={t("ui.editor.file.caption_placeholder")} />
        {#if captionError}
          <p role="alert" class="m-0 text-danger">{tError(captionError.code, captionError.params)}</p>
        {:else if file.caption !== ""}
          <p class="m-0 text-muted">{t("ui.editor.preview_now")} <span class="text-text">{captionPreview}</span></p>
        {:else}
          <p class="m-0 text-[12px] text-muted">{t("ui.editor.file.caption_empty")}</p>
        {/if}
        {@render err("caption")}
      {/if}
    </div>

    <div class="flex flex-col gap-1.5">
      <label for="{uid}-send" class="text-muted">{t("ui.filespec.send_name")}</label>
      <input id="{uid}-send" class="input mono" bind:value={file.send_name} placeholder={t("ui.editor.file.send_name_placeholder")} />
      <p class="m-0 text-[12px] text-muted">{t("ui.filespec.send_name.hint")}</p>
      {#if nameError}
        <p role="alert" class="m-0 text-danger">{tError(nameError.code, nameError.params)}</p>
      {:else if names.length > 0 && names[0] !== ""}
        <p class="m-0 text-muted" data-testid="send-name-preview">
          {t("ui.editor.file.recipients_see")} <span class="text-text">{names.join(", ")}</span>
        </p>
      {/if}
      {@render err("send_name")}
    </div>
  </div>
</li>
