<script lang="ts">
  // WhatsApp recipient picker: a number (checked with ValidateTarget when the
  // channel is connected; otherwise kept unverified, checked at the next
  // connection) or a group from the account's groups (needs the connection).
  import SegmentedControl from "../../components/SegmentedControl.svelte";
  import { api, ChannelStates, fromRef, Verified, type AppError, type Target } from "../../lib/api";
  import { t, tError } from "../../lib/i18n.svelte";
  import type { TargetPickerProps } from "..";

  let { channel, onpick, oncancel }: TargetPickerProps = $props();

  type Tab = "number" | "group";
  let tab = $state<Tab>("number");
  let number = $state("");
  let busy = $state(false);
  let error = $state<AppError | null>(null);

  let groups = $state<Target[] | null>(null);
  let groupsError = $state<AppError | null>(null);
  let search = $state("");

  const connected = $derived(channel.status.state === ChannelStates.StateConnected);
  const uid = `wa-${Math.random().toString(36).slice(2)}`;

  const shown = $derived.by(() => {
    const q = search.trim().toLowerCase();
    return (groups ?? []).filter((g) => q === "" || g.display_name.toLowerCase().includes(q));
  });

  async function loadGroups() {
    groupsError = null;
    const r = await api.listTargets(channel.id);
    if (r.ok) groups = r.value;
    else groupsError = r.error;
  }

  $effect(() => {
    if (tab === "group" && connected && groups === null && groupsError === null) void loadGroups();
  });

  async function addNumber(e: SubmitEvent) {
    e.preventDefault();
    const typed = number.trim();
    if (typed === "") return;
    busy = true;
    error = null;
    const raw = { kind: "number", number: typed };
    const r = await api.validateTarget(channel.id, raw);
    busy = false;
    if (r.ok) {
      onpick({
        target: r.value.target,
        display_name: r.value.display_name || typed,
        verified: r.value.verified,
        warning: fromRef(r.value.warning),
      });
    } else if (r.error.code === "channel.not_connected") {
      // The channel isn't open: keep the number unverified, checked when it connects.
      onpick({
        target: raw,
        display_name: typed,
        verified: Verified.VerifiedUnknown,
        warning: { code: "recipient.check_unavailable", params: { channel: channel.name } },
      });
    } else {
      error = r.error;
    }
  }
</script>

<div class="flex flex-col gap-3">
  <SegmentedControl
    label={t("provider.whatsapp.target.kind")}
    options={[
      { value: "number", label: t("provider.whatsapp.target.kind.number") },
      { value: "group", label: t("provider.whatsapp.target.kind.group") },
    ]}
    value={tab}
    onchange={(v) => (tab = v)}
  />

  {#if tab === "number"}
    <form class="flex flex-col gap-1.5" onsubmit={addNumber}>
      <label for="{uid}-number" class="text-muted">{t("provider.whatsapp.target.number")}</label>
      <div class="flex flex-wrap gap-2">
        <input
          id="{uid}-number"
          class="input w-[240px] tabular-nums"
          type="tel"
          placeholder="+39 333 1234567"
          maxlength="32"
          bind:value={number}
          aria-describedby={error ? `${uid}-error` : undefined}
        />
        <button type="submit" class="btn btn-primary" disabled={busy || number.trim() === ""}>{t("ui.recipients.add_this")}</button>
        <button type="button" class="btn" onclick={oncancel}>{t("ui.dialog.cancel")}</button>
      </div>
      {#if !connected}
        <p class="m-0 text-[12px] text-warning">{t("provider.whatsapp.target.offline_note", { channel: channel.name })}</p>
      {/if}
      {#if error}
        <p id="{uid}-error" role="alert" class="m-0 text-danger">{tError(error.code, { channel: channel.name, ...error.params })}</p>
      {/if}
    </form>
  {:else if !connected}
    <p class="m-0 text-warning">{tError("channel.not_connected", { channel: channel.name })}</p>
    <div><button type="button" class="btn" onclick={oncancel}>{t("ui.dialog.cancel")}</button></div>
  {:else}
    <div class="flex flex-col gap-2">
      <label for="{uid}-search" class="text-muted">{t("provider.whatsapp.target.search")}</label>
      <div class="flex gap-2">
        <input id="{uid}-search" class="input w-[280px]" type="search" bind:value={search} />
        <button type="button" class="btn" onclick={oncancel}>{t("ui.dialog.cancel")}</button>
      </div>
      {#if groupsError}
        <p role="alert" class="m-0 text-danger">{tError(groupsError.code, { channel: channel.name, ...groupsError.params })}</p>
        <div><button type="button" class="btn" onclick={loadGroups}>{t("ui.retry")}</button></div>
      {:else if groups === null}
        <p class="m-0 text-muted" aria-busy="true">{t("provider.whatsapp.target.loading")}</p>
      {:else if shown.length === 0}
        <p class="m-0 text-muted">{t("provider.whatsapp.target.no_groups")}</p>
      {:else}
        <ul
          class="m-0 flex max-h-[240px] list-none flex-col overflow-y-auto rounded-lg border border-divider p-0"
          aria-label={t("provider.whatsapp.target.groups")}
        >
          {#each shown as g (JSON.stringify(g.target))}
            <li class="border-b border-divider last:border-b-0">
              <button
                type="button"
                class="w-full cursor-pointer border-0 bg-transparent px-3 py-2 text-left text-text hover:bg-nav-active"
                onclick={() => onpick({ target: g.target, display_name: g.display_name, verified: Verified.VerifiedYes, warning: null })}
                >{g.display_name}</button
              >
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>
