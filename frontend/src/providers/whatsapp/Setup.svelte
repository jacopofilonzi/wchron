<script lang="ts">
  // WhatsApp pairing: the terms-of-service note, the steps on the phone, the
  // live QR code (from channel:qr) and the outcome. The page owns the calls;
  // this component only shows the pairing and asks for start/cancel/close.
  import QrCode from "../../components/QrCode.svelte";
  import { ChannelStates, PairKind, type ChannelInfo } from "../../lib/api";
  import { t, tError } from "../../lib/i18n.svelte";
  import type { PairingView } from "../../lib/pairing.svelte";

  let {
    channel,
    view,
    onstart,
    oncancel,
    onclose,
  }: {
    channel: ChannelInfo;
    view: PairingView | undefined;
    onstart: () => void;
    oncancel: () => void;
    onclose: () => void;
  } = $props();

  const running = $derived(channel.pairing || (view !== undefined && view.result === null));
  const linked = $derived(view?.result === PairKind.PairSuccess);
  const done = $derived(linked && channel.status.state === ChannelStates.StateConnected);
</script>

<div class="flex flex-col gap-4">
  <p class="m-0 rounded-lg border border-warning px-3 py-2 text-[13px] text-text-soft">
    {t("provider.whatsapp.tos")}
  </p>

  {#if done}
    <p role="status" class="m-0 text-sm text-success">{t("provider.whatsapp.pair.done", { account: channel.status.account })}</p>
    <div><button type="button" class="btn" onclick={onclose}>{t("provider.whatsapp.pair.close")}</button></div>
  {:else if linked}
    <p role="status" class="m-0 text-sm">{t("provider.whatsapp.pair.success")}</p>
  {:else if running}
    <div class="flex flex-wrap items-start gap-6">
      {#if view?.code}
        <QrCode text={view.code} label={t("provider.whatsapp.pair.qr_label")} />
      {:else}
        <div class="flex size-[232px] items-center justify-center rounded-lg border border-divider text-[13px] text-muted" aria-busy="true">
          {t("provider.whatsapp.pair.waiting")}
        </div>
      {/if}
      <div class="flex max-w-[320px] flex-col gap-3">
        <ol class="m-0 flex list-decimal flex-col gap-1.5 pl-5 text-sm">
          <li>{t("provider.whatsapp.pair.step1")}</li>
          <li>{t("provider.whatsapp.pair.step2")}</li>
          <li>{t("provider.whatsapp.pair.step3")}</li>
        </ol>
        <p class="m-0 text-[13px] text-muted">{t("provider.whatsapp.pair.expire")}</p>
        <div><button type="button" class="btn" onclick={oncancel}>{t("provider.whatsapp.pair.cancel")}</button></div>
      </div>
    </div>
  {:else}
    {#if view?.error}
      <p role="alert" class="m-0 text-danger">{tError(view.error.code, view.error.params)}</p>
    {:else if view?.result === PairKind.PairCancelled}
      <p role="status" class="m-0 text-muted">{t("provider.whatsapp.pair.cancelled")}</p>
    {/if}
    <div class="flex gap-2">
      <button type="button" class="btn btn-primary" onclick={onstart}>{t("provider.whatsapp.pair.start")}</button>
      <button type="button" class="btn" onclick={onclose}>{t("provider.whatsapp.pair.close")}</button>
    </div>
  {/if}
</div>
