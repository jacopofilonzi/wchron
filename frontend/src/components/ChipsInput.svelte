<script lang="ts">
  // A list of short values as chips (sheet names, fixed times): type and
  // press Enter (or Add) to add one, × to remove it. `preview` shows what a
  // value becomes (tokens rendered now) next to it.
  import { t } from "../lib/i18n.svelte";

  let {
    id,
    values = $bindable(),
    label,
    placeholder = "",
    preview = {},
    mono = false,
    maxlength = 120,
  }: {
    id: string;
    values: string[];
    label: string;
    placeholder?: string;
    preview?: Record<string, string>;
    mono?: boolean;
    maxlength?: number;
  } = $props();

  let draft = $state("");

  function add() {
    const v = draft.trim();
    if (v === "" || values.includes(v)) {
      draft = "";
      return;
    }
    values = [...values, v];
    draft = "";
  }
</script>

<div class="flex flex-wrap items-center gap-1.5">
  <ul class="m-0 flex list-none flex-wrap gap-1.5 p-0" aria-label={label}>
    {#each values as v, i (v)}
      <li class="chip">
        <span class={mono ? "mono" : ""}>{v}</span>
        {#if preview[v] !== undefined && preview[v] !== v}<span class="text-muted">→ {preview[v]}</span>{/if}
        <button
          type="button"
          class="icon-btn size-5"
          aria-label={t("ui.chips.remove", { value: v })}
          onclick={() => (values = values.filter((_, j) => j !== i))}>×</button
        >
      </li>
    {/each}
  </ul>
  <input
    {id}
    class="input h-8 w-[180px] {mono ? 'mono' : ''}"
    {placeholder}
    {maxlength}
    aria-label={label}
    bind:value={draft}
    onkeydown={(e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        add();
      }
    }}
  />
  <button type="button" class="btn h-8" aria-label={t("ui.chips.add_to", { label })} disabled={draft.trim() === ""} onclick={add}>{t("ui.chips.add")}</button>
</div>
