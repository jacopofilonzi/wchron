<script lang="ts">
  // Channels: the configured accounts with their live status (from the
  // Snapshot), add (provider + name, then its setup), pairing, rename, log
  // out and delete. Log out, re-pair and delete act for real on the account
  // and ask for a confirmation that names the channel first.
  import ConfirmDialog from "../components/ConfirmDialog.svelte";
  import {
    api,
    ChannelStates,
    fromRef,
    PairKind,
    type AppError,
    type ChannelInfo,
    type ProviderInfo,
    type Usage,
  } from "../lib/api";
  import { t, tError, type Key } from "../lib/i18n.svelte";
  import { clearPairing, pairings, pairingStarted } from "../lib/pairing.svelte";
  import { app } from "../lib/state.svelte";
  import { providerLabel, providerUIs } from "../providers";

  let providers = $state<ProviderInfo[]>([]);
  let loadError = $state<AppError | null>(null);

  let adding = $state(false);
  let newProvider = $state("");
  let newName = $state("");
  let addError = $state<AppError | null>(null);
  let addBusy = $state(false);

  let setupFor = $state<string | null>(null);
  let renaming = $state<{ id: string; name: string } | null>(null);
  let rowErrors = $state<Record<string, AppError>>({});
  let notice = $state("");

  type Confirm = { kind: "delete" | "logout" | "repair"; channel: ChannelInfo; usage: Usage | null };
  let confirm = $state<Confirm | null>(null);
  let confirmBusy = $state(false);

  const channels = $derived(app.s?.channel_list ?? []);

  $effect(() => {
    void api.providers().then((r) => {
      if (r.ok) {
        providers = r.value;
        newProvider = r.value[0]?.type ?? "";
      } else {
        loadError = r.error;
      }
    });
  });

  function caps(type: string) {
    return providers.find((p) => p.type === type)?.capabilities;
  }

  const stateKeys: Record<string, Key> = {
    [ChannelStates.StateConnected]: "ui.channels.state.connected",
    [ChannelStates.StateConnecting]: "ui.channels.state.connecting",
    [ChannelStates.StateDisconnected]: "ui.channels.state.disconnected",
    [ChannelStates.StateLoggedOut]: "ui.channels.state.logged_out",
    [ChannelStates.StateNotPaired]: "ui.channels.state.not_paired",
    [ChannelStates.StateOutdated]: "ui.channels.state.outdated",
    [ChannelStates.StateError]: "ui.channels.state.error",
  };

  function stateLabel(c: ChannelInfo): string {
    if (c.pairing) return t("ui.channels.state.pairing");
    const k = stateKeys[c.status.state];
    return k ? t(k) : c.status.state;
  }

  function dotClass(c: ChannelInfo): string {
    if (c.pairing) return "bg-accent";
    switch (c.status.state) {
      case ChannelStates.StateConnected:
        return "bg-success";
      case ChannelStates.StateConnecting:
      case ChannelStates.StateNotPaired:
      case ChannelStates.StateLoggedOut:
        return "bg-warning";
      case ChannelStates.StateOutdated:
      case ChannelStates.StateError:
        return "bg-danger";
      default:
        return "bg-idle";
    }
  }

  function setError(id: string, e: AppError | null) {
    if (e) rowErrors[id] = e;
    else delete rowErrors[id];
  }

  async function startPair(id: string) {
    setupFor = id;
    pairingStarted(id);
    const r = await api.startPairing(id);
    if (!r.ok) pairings[id] = { code: "", result: PairKind.PairFailed, error: r.error };
  }

  async function cancelPair(id: string) {
    await api.cancelPairing(id);
  }

  function closeSetup(id: string) {
    const c = channels.find((x) => x.id === id);
    if (c?.pairing) void api.cancelPairing(id);
    clearPairing(id);
    setupFor = null;
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    addBusy = true;
    const r = await api.createChannel(newProvider, newName);
    addBusy = false;
    if (!r.ok) {
      addError = r.error;
      return;
    }
    addError = null;
    adding = false;
    newName = "";
    if (caps(r.value.provider)?.pairing) await startPair(r.value.id);
  }

  async function saveRename(e: SubmitEvent) {
    e.preventDefault();
    if (!renaming) return;
    const { id, name } = renaming;
    const r = await api.renameChannel(id, name);
    setError(id, r.ok ? null : r.error);
    if (r.ok) renaming = null;
  }

  async function askDelete(c: ChannelInfo) {
    const r = await api.channelUsage(c.id);
    if (!r.ok) {
      setError(c.id, r.error);
      return;
    }
    confirm = { kind: "delete", channel: c, usage: r.value };
  }

  async function confirmed() {
    if (!confirm) return;
    const { kind, channel: c } = confirm;
    confirmBusy = true;
    notice = "";
    if (kind === "delete") {
      const r = await api.deleteChannel(c.id);
      setError(c.id, r.ok ? null : r.error);
      if (r.ok) {
        clearPairing(c.id);
        if (setupFor === c.id) setupFor = null;
        if (r.value.was_paired && !r.value.logged_out) notice = t("ui.channels.delete.not_logged_out", { name: c.name });
      }
    } else if (kind === "logout") {
      const r = await api.logout(c.id);
      setError(c.id, r.ok ? null : r.error);
    } else {
      setError(c.id, null);
      void startPair(c.id);
    }
    confirmBusy = false;
    confirm = null;
  }

  const confirmTexts = {
    delete: { title: "ui.channels.delete.title", ask: "ui.channels.delete.confirm", button: "ui.channels.delete" },
    logout: { title: "ui.channels.logout.title", ask: "ui.channels.logout.confirm", button: "ui.channels.logout" },
    repair: { title: "ui.channels.repair.title", ask: "ui.channels.repair.confirm", button: "ui.channels.repair" },
  } as const;
</script>

<div class="flex items-center justify-between gap-4">
  <h1 class="page-title">{t("ui.channels.title")}</h1>
  {#if !adding}
    <button type="button" class="btn btn-primary" disabled={providers.length === 0} onclick={() => (adding = true)}>
      {t("ui.channels.add")}
    </button>
  {/if}
</div>

{#if loadError}<p role="alert" class="m-0 text-danger">{tError(loadError.code, loadError.params)}</p>{/if}
{#if notice}<p role="status" class="card m-0 px-4 py-3 text-[13px] text-warning">{notice}</p>{/if}

{#if adding}
  <form class="card flex flex-col gap-4 p-5 text-[13px]" aria-labelledby="add-title" onsubmit={create}>
    <h2 id="add-title" class="m-0 text-[15px] font-semibold">{t("ui.channels.add.title")}</h2>
    <div class="flex flex-wrap gap-4">
      <label class="flex flex-col gap-1.5">
        <span class="font-medium">{t("ui.channels.add.provider")}</span>
        <select class="field" bind:value={newProvider}>
          {#each providers as p (p.type)}<option value={p.type}>{providerLabel(p.type, t)}</option>{/each}
        </select>
      </label>
      <label class="flex min-w-[260px] grow flex-col gap-1.5">
        <span class="font-medium">{t("ui.channels.add.name")}</span>
        <input class="input" bind:value={newName} placeholder={t("ui.channels.add.name_placeholder")} maxlength="80" />
      </label>
    </div>
    {#if addError}<p role="alert" class="m-0 text-danger">{tError(addError.code, addError.params)}</p>{/if}
    <div class="flex gap-2">
      <button type="submit" class="btn btn-primary" disabled={addBusy}>{t("ui.channels.add.create")}</button>
      <button
        type="button"
        class="btn"
        onclick={() => {
          adding = false;
          addError = null;
        }}>{t("ui.dialog.cancel")}</button
      >
    </div>
  </form>
{/if}

{#if channels.length === 0 && !adding}
  <div class="card px-6 py-10 text-center text-[13px] text-muted">{t("ui.channels.empty")}</div>
{:else if channels.length > 0}
  <ul class="card m-0 list-none p-0 text-[13px]" aria-label={t("ui.channels.list")}>
    {#each channels as c (c.id)}
      {@const paired = c.status.account !== ""}
      {@const pairable = caps(c.provider)?.pairing ?? false}
      {@const ui = providerUIs[c.provider]}
      {@const reason = fromRef(c.status.error)}
      <li class="flex flex-col gap-3 border-b border-divider px-[18px] py-4 last:border-b-0">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
          <div class="min-w-0 grow">
            {#if renaming?.id === c.id}
              <form class="flex items-center gap-2" onsubmit={saveRename}>
                <input
                  class="field"
                  aria-label={t("ui.channels.add.name")}
                  bind:value={renaming.name}
                  maxlength="80"
                />
                <button type="submit" class="btn btn-primary">{t("ui.channels.save")}</button>
                <button type="button" class="btn" onclick={() => (renaming = null)}>{t("ui.dialog.cancel")}</button>
              </form>
            {:else}
              <div class="text-sm font-medium">{c.name}</div>
            {/if}
            <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-muted">
              <span>{providerLabel(c.provider, t)}</span>
              <span class="flex items-center gap-1.5 text-text-soft">
                <span class="status-dot {dotClass(c)}" aria-hidden="true"></span>{stateLabel(c)}
              </span>
              {#if paired}<span>{t("ui.channels.account", { account: c.status.account })}</span>{/if}
            </div>
          </div>
          <div class="flex flex-wrap gap-2">
            {#if pairable && !paired && setupFor !== c.id}
              <button type="button" class="btn btn-primary" onclick={() => startPair(c.id)}>{t("ui.channels.pair")}</button>
            {/if}
            {#if pairable && paired}
              <button type="button" class="btn" onclick={() => (confirm = { kind: "repair", channel: c, usage: null })}>
                {t("ui.channels.repair")}
              </button>
              <button type="button" class="btn" onclick={() => (confirm = { kind: "logout", channel: c, usage: null })}>
                {t("ui.channels.logout")}
              </button>
            {/if}
            <button type="button" class="btn" onclick={() => (renaming = { id: c.id, name: c.name })}>{t("ui.channels.rename")}</button>
            <button type="button" class="btn" onclick={() => askDelete(c)}>{t("ui.channels.delete")}</button>
          </div>
        </div>
        {#if reason && !c.pairing}
          <p role="alert" class="m-0 text-danger">{tError(reason.code, reason.params)}</p>
        {/if}
        {#if rowErrors[c.id]}
          <p role="alert" class="m-0 text-danger">{tError(rowErrors[c.id]!.code, rowErrors[c.id]!.params)}</p>
        {/if}
        {#if setupFor === c.id && ui}
          <div class="rounded-lg border border-divider p-4">
            <ui.setup
              channel={c}
              view={pairings[c.id]}
              onstart={() => startPair(c.id)}
              oncancel={() => cancelPair(c.id)}
              onclose={() => closeSetup(c.id)}
            />
          </div>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

{#if confirm}
  {@const texts = confirmTexts[confirm.kind]}
  {@const c = confirm.channel}
  <ConfirmDialog
    title={t(texts.title)}
    confirmLabel={t(texts.button)}
    busy={confirmBusy}
    onconfirm={confirmed}
    oncancel={() => (confirm = null)}
  >
    <p class="m-0 text-sm">{t(texts.ask, { name: c.name })}</p>
    {#if confirm.kind === "delete"}
      <p class="m-0 text-text-soft">{t("ui.channels.delete.consequences")}</p>
      {@const affected = confirm.usage?.schedules ?? []}
      {#if affected.length > 0}
        <p class="m-0 text-text-soft">{t("ui.channels.delete.schedules")}</p>
        <ul class="m-0 flex flex-col gap-1 pl-5">
          {#each affected as s (s.id)}
            <li>
              {s.disabled
                ? t("ui.channels.delete.schedule_disabled", { name: s.name })
                : t("ui.channels.delete.schedule", { name: s.name, count: String(s.recipients) })}
            </li>
          {/each}
        </ul>
      {/if}
      {#if confirm.usage && confirm.usage.error_recipients > 0}
        <p class="m-0 text-text-soft">{t("ui.channels.delete.error_recipients", { count: String(confirm.usage.error_recipients) })}</p>
      {/if}
    {:else if confirm.kind === "logout"}
      <p class="m-0 text-text-soft">{t("ui.channels.logout.consequences")}</p>
    {:else}
      <p class="m-0 text-text-soft">{t("ui.channels.repair.consequences")}</p>
    {/if}
  </ConfirmDialog>
{/if}
