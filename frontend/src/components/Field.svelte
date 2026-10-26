<script lang="ts">
  // A labelled form field: label, the control (children, with id = the
  // field's id), an optional hint and the field's error. The control should
  // set aria-describedby={describedBy(id)} so the error is announced.
  import type { Snippet } from "svelte";
  import type { AppError } from "../lib/api";
  import { tError } from "../lib/i18n.svelte";

  let {
    id,
    label,
    hint = "",
    error = null,
    errorParams = {},
    class: cls = "",
    children,
  }: {
    id: string;
    label: string;
    hint?: string;
    error?: AppError | null;
    /** Extra params for the error text (e.g. `schedule` in the editor). */
    errorParams?: Record<string, string>;
    class?: string;
    children: Snippet;
  } = $props();
</script>

<div class="flex min-w-0 flex-col gap-1.5 {cls}">
  <label for={id} class="text-muted">{label}</label>
  {@render children()}
  {#if hint}<p id="{id}-hint" class="m-0 text-[12px] text-muted">{hint}</p>{/if}
  {#if error}<p id="{id}-error" role="alert" class="m-0 text-danger">{tError(error.code, { ...errorParams, ...error.params })}</p>{/if}
</div>
