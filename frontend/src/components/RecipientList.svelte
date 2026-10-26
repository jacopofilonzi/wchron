<script lang="ts">
  // The recipients of a schedule (or the error recipients): name, channel and
  // kind, the check badge, remove (the page asks for the confirmation).
  import Badge from "./Badge.svelte";
  import { Verified, type AppError, type ChannelInfo, type Recipient } from "../lib/api";
  import { t, tError } from "../lib/i18n.svelte";
  import { providerUIs } from "../providers";

  let {
    recipients,
    channels,
    label,
    errors = {},
    errorParams = {},
    onremove,
  }: {
    recipients: Recipient[];
    channels: ChannelInfo[];
    label: string;
    /** Problems by recipient id. */
    errors?: Record<string, AppError>;
    errorParams?: Record<string, string>;
    onremove: (r: Recipient) => void;
  } = $props();

  function channelOf(r: Recipient) {
    return channels.find((c) => c.id === r.channel_id);
  }
</script>

{#if recipients.length > 0}
  <ul class="m-0 flex list-none flex-col gap-2 p-0" aria-label={label}>
    {#each recipients as r (r.id)}
      {@const ch = channelOf(r)}
      {@const kind = ch ? providerUIs[ch.provider]?.targetKind(r.target) : null}
      <li class="flex flex-col gap-1 rounded-lg border border-control-border px-3 py-2">
        <div class="flex items-center gap-3">
          <div class="min-w-0 grow">
            <div class="truncate font-medium">{r.display_name}</div>
            <div class="truncate text-[12px] text-muted">
              {ch ? ch.name : t("ui.recipients.channel_missing")}{kind ? ` · ${t(kind)}` : ""}
            </div>
          </div>
          {#if r.verified === Verified.VerifiedYes}
            <Badge tone="success" text={t("ui.recipients.verified")} />
          {:else if r.verified === Verified.VerifiedNo}
            <Badge tone="danger" text={t("ui.recipients.invalid")} />
          {:else}
            <Badge tone="warning" text={t("ui.recipients.unverified")} />
          {/if}
          <button type="button" class="btn" aria-label={t("ui.recipients.remove_named", { name: r.display_name })} onclick={() => onremove(r)}>
            {t("ui.recipients.remove")}
          </button>
        </div>
        {#if r.verified === Verified.VerifiedUnknown}
          <p class="m-0 text-[12px] text-muted">{t("ui.recipients.unverified_note")}</p>
        {:else if r.verified === Verified.VerifiedNo}
          <p class="m-0 text-[12px] text-danger">{t("ui.recipients.invalid_note")}</p>
        {/if}
        {#if errors[r.id]}
          <p role="alert" class="m-0 text-danger">{tError(errors[r.id]!.code, { ...errorParams, ...errors[r.id]!.params })}</p>
        {/if}
      </li>
    {/each}
  </ul>
{/if}
