<script lang="ts">
  // Adds a recipient: choose the channel, then the provider's own target
  // picker (providers map); the result becomes a Recipient with a new id.
  // The core never looks inside the target.
  import { api, type AppError, type ChannelInfo, type Recipient } from "../lib/api";
  import { t, tError } from "../lib/i18n.svelte";
  import { providerLabel, providerUIs, type PickedTarget } from "../providers";

  let {
    channels,
    onadd,
    oncancel,
  }: {
    channels: ChannelInfo[];
    onadd: (r: Recipient) => void;
    oncancel: () => void;
  } = $props();

  // Only channels whose provider has a picker can take recipients here.
  const usable = $derived(channels.filter((c) => providerUIs[c.provider]));
  let channelID = $state("");
  let error = $state<AppError | null>(null);
  const uid = `rp-${Math.random().toString(36).slice(2)}`;

  $effect(() => {
    if (!usable.some((c) => c.id === channelID)) channelID = usable[0]?.id ?? "";
  });

  const channel = $derived(usable.find((c) => c.id === channelID));
  const Picker = $derived(channel ? providerUIs[channel.provider]?.targetPicker : undefined);

  async function picked(p: PickedTarget) {
    if (!channel) return;
    const id = await api.newRecipientID();
    if (!id.ok) {
      error = id.error;
      return;
    }
    onadd({ id: id.value, channel_id: channel.id, target: p.target, display_name: p.display_name, verified: p.verified });
  }
</script>

<div class="flex flex-col gap-3 rounded-lg border border-divider p-4" role="group" aria-label={t("ui.recipients.add")}>
  {#if usable.length === 0}
    <p class="m-0 text-muted">{t("ui.recipients.no_channels")}</p>
    <div class="flex gap-2">
      <a class="btn no-underline" href="#/channels">{t("ui.recipients.go_channels")}</a>
      <button type="button" class="btn" onclick={oncancel}>{t("ui.dialog.cancel")}</button>
    </div>
  {:else}
    {#if usable.length > 1}
      <div class="flex flex-col gap-1.5">
        <label for="{uid}-channel" class="text-muted">{t("ui.recipients.channel")}</label>
        <select id="{uid}-channel" class="field w-[280px]" bind:value={channelID}>
          {#each usable as c (c.id)}<option value={c.id}>{c.name} · {providerLabel(c.provider, t)}</option>{/each}
        </select>
      </div>
    {:else if channel}
      <p class="m-0 text-muted">{t("ui.recipients.on_channel", { channel: channel.name })}</p>
    {/if}
    {#if channel && Picker}
      {#key channel.id}
        <Picker {channel} onpick={picked} {oncancel} />
      {/key}
    {/if}
    {#if error}<p role="alert" class="m-0 text-danger">{tError(error.code, error.params)}</p>{/if}
  {/if}
</div>
