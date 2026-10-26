<script lang="ts">
  // A modal confirmation (native <dialog>: focus trap, Escape closes) for
  // permanent or real-world actions: it names the item and states the
  // consequences (AGENTS.md "Deleting is permanent").
  import type { Snippet } from "svelte";
  import { t } from "../lib/i18n.svelte";

  let {
    title,
    confirmLabel,
    busy = false,
    onconfirm,
    oncancel,
    children,
  }: {
    title: string;
    confirmLabel: string;
    busy?: boolean;
    onconfirm: () => void;
    oncancel: () => void;
    children: Snippet;
  } = $props();

  let dialog = $state<HTMLDialogElement | null>(null);
  const titleId = `confirm-${Math.random().toString(36).slice(2)}`;

  $effect(() => {
    dialog?.showModal();
    return () => dialog?.close();
  });
</script>

<dialog
  bind:this={dialog}
  aria-labelledby={titleId}
  class="card m-auto w-[440px] max-w-[calc(100vw-32px)] p-0 text-[13px] text-text backdrop:bg-backdrop"
  oncancel={(e) => {
    e.preventDefault();
    if (!busy) oncancel();
  }}
>
  <div class="flex flex-col gap-3 px-5 pt-5 pb-4">
    <h2 id={titleId} class="m-0 text-[15px] font-semibold">{title}</h2>
    {@render children()}
  </div>
  <div class="flex justify-end gap-2 border-t border-divider px-5 py-3">
    <button type="button" class="btn" disabled={busy} onclick={oncancel}>{t("ui.dialog.cancel")}</button>
    <button type="button" class="btn btn-danger" disabled={busy} onclick={onconfirm}>{confirmLabel}</button>
  </div>
</dialog>
