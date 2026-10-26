// The per-provider UI, by provider type: the only place in the frontend that
// names a provider (AGENTS.md "The core never knows a provider"). A provider
// without an entry still shows in the channel list, without a setup.
import type { Component } from "svelte";
import type { ChannelInfo } from "../lib/api";
import type { Key } from "../lib/i18n.svelte";
import type { PairingView } from "../lib/pairing.svelte";
import WhatsAppSetup from "./whatsapp/Setup.svelte";

export interface SetupProps {
  channel: ChannelInfo;
  view: PairingView | undefined;
  onstart: () => void;
  oncancel: () => void;
  onclose: () => void;
}

export interface ProviderUI {
  /** i18n key of the provider's name. */
  label: Key;
  /** The setup panel (pairing), for providers with Capabilities.pairing. */
  setup: Component<SetupProps>;
}

export const providerUIs: Record<string, ProviderUI> = {
  whatsapp: { label: "provider.whatsapp.name", setup: WhatsAppSetup },
};

/** The provider's display name, or its type when it has no UI entry. */
export function providerLabel(type: string, t: (k: Key) => string): string {
  const ui = providerUIs[type];
  return ui ? t(ui.label) : type;
}
