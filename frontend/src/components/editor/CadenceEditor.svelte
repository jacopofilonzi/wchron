<script lang="ts">
  // "When": interval (every h:m from an anchor, optional window) or fixed
  // times, plus the active days. The values are checked in Go: `problem`
  // gives the save problems by field, `live` the error of the live preview
  // (PreviewRuns), shown under the field its code belongs to.
  import ChipsInput from "../ChipsInput.svelte";
  import SegmentedControl from "../SegmentedControl.svelte";
  import TimeInput from "../TimeInput.svelte";
  import { CadenceMode, type AppError, type Cadence } from "../../lib/api";
  import { dayLabel, weekdays } from "../../lib/format";
  import { t, tError } from "../../lib/i18n.svelte";

  let {
    cadence = $bindable(),
    problem,
    live,
  }: {
    cadence: Cadence;
    problem: (field: string) => AppError | null;
    live: AppError | null;
  } = $props();

  // Display only: which field a live cadence error goes under (the save
  // problems come addressed from Go).
  const liveFields: Record<string, string> = {
    "cadence.interval_invalid": "interval",
    "cadence.anchor_invalid": "anchor",
    "cadence.window_invalid": "window",
    "cadence.window_empty": "window",
    "cadence.no_times": "times",
    "cadence.time_invalid": "times",
    "cadence.time_duplicate": "times",
    "schedule.no_days": "days",
  };

  function errorFor(field: string): AppError | null {
    const saved = problem(`cadence.${field}`);
    if (saved) return saved;
    return live && liveFields[live.code] === field ? live : null;
  }

  const interval = $derived(cadence.mode === CadenceMode.CadenceInterval);
  const overnight = $derived(cadence.window !== null && cadence.window.start > cadence.window.end && /^\d\d:\d\d$/.test(cadence.window.end));

  function toggleDay(d: string) {
    const days = cadence.days ?? [];
    cadence.days = days.includes(d) ? days.filter((x) => x !== d) : weekdays.filter((w) => w === d || days.includes(w));
  }

  function setWindow(on: boolean) {
    cadence.window = on ? { start: "08:00", end: "20:00" } : null;
  }
</script>

{#snippet err(field: string)}
  {@const e = errorFor(field)}
  {#if e}<p id="cad-{field}-error" role="alert" class="m-0 text-danger">{tError(e.code, e.params)}</p>{/if}
{/snippet}

<section class="section-card" aria-labelledby="when-title">
  <h2 id="when-title" class="section-title">{t("ui.editor.when")}</h2>
  <SegmentedControl
    label={t("ui.editor.when.mode")}
    options={[
      { value: CadenceMode.CadenceInterval, label: t("ui.editor.when.interval") },
      { value: CadenceMode.CadenceFixed, label: t("ui.editor.when.fixed") },
    ]}
    value={cadence.mode}
    onchange={(m) => (cadence.mode = m)}
  />

  {#if interval}
    <div class="flex flex-wrap items-end gap-4">
      <fieldset class="m-0 flex flex-col gap-1.5 border-0 p-0">
        <legend class="mb-1.5 p-0 text-muted">{t("ui.editor.when.every")}</legend>
        <div class="flex items-center gap-2">
          <input
            id="cad-hours"
            class="input w-[64px] tabular-nums"
            type="number"
            min="0"
            max="24"
            aria-label={t("ui.editor.when.hours")}
            aria-invalid={errorFor("interval") !== null}
            bind:value={cadence.interval.hours}
          />
          <span class="text-muted">{t("ui.editor.when.h")}</span>
          <input
            id="cad-minutes"
            class="input w-[64px] tabular-nums"
            type="number"
            min="0"
            max="59"
            aria-label={t("ui.editor.when.minutes")}
            aria-invalid={errorFor("interval") !== null}
            bind:value={cadence.interval.minutes}
          />
          <span class="text-muted">{t("ui.editor.when.min")}</span>
        </div>
      </fieldset>
      <div class="flex flex-col gap-1.5">
        <label for="cad-anchor" class="text-muted">{t("ui.editor.when.anchor")}</label>
        <TimeInput id="cad-anchor" bind:value={cadence.anchor} invalid={errorFor("anchor") !== null} />
      </div>
      <label class="flex h-9 items-center gap-2">
        <input type="checkbox" checked={cadence.window !== null} onchange={(e) => setWindow(e.currentTarget.checked)} />
        {t("ui.editor.when.window")}
      </label>
      {#if cadence.window}
        <div class="flex flex-col gap-1.5">
          <label for="cad-from" class="text-muted">{t("ui.editor.when.from")}</label>
          <TimeInput id="cad-from" bind:value={cadence.window.start} invalid={errorFor("window") !== null} />
        </div>
        <div class="flex flex-col gap-1.5">
          <label for="cad-to" class="text-muted">{t("ui.editor.when.to")}</label>
          <TimeInput id="cad-to" bind:value={cadence.window.end} invalid={errorFor("window") !== null} />
        </div>
      {/if}
    </div>
    {@render err("interval")}
    {@render err("anchor")}
    {@render err("window")}
    {#if overnight}<p class="m-0 text-[12px] text-muted">{t("ui.editor.when.overnight")}</p>{/if}
    <p class="m-0 text-[12px] text-muted">{t("ui.editor.when.interval_hint")}</p>
  {:else}
    <div class="flex flex-col gap-1.5">
      <label for="cad-times" class="text-muted">{t("ui.editor.when.times")}</label>
      <ChipsInput id="cad-times" label={t("ui.editor.when.times")} placeholder="HH:MM" maxlength={5} bind:values={() => cadence.times ?? [], (v) => (cadence.times = v)} />
    </div>
    {@render err("times")}
  {/if}

  <div class="flex flex-wrap items-center gap-1.5" role="group" aria-label={t("ui.editor.when.days")}>
    {#each weekdays as d (d)}
      <button type="button" class="chip-toggle" aria-pressed={(cadence.days ?? []).includes(d)} onclick={() => toggleDay(d)}>{dayLabel(d)}</button>
    {/each}
    <span class="grow"></span>
    <button type="button" class="link-btn" onclick={() => (cadence.days = ["mon", "tue", "wed", "thu", "fri"])}>{t("ui.editor.when.weekdays")}</button>
    <span class="text-muted" aria-hidden="true">·</span>
    <button type="button" class="link-btn" onclick={() => (cadence.days = [...weekdays])}>{t("ui.editor.when.all_days")}</button>
  </div>
  {@render err("days")}
</section>
