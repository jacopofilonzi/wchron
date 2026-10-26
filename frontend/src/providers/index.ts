// The per-provider UI, by provider type: the only place in the frontend that
// names a provider (AGENTS.md "The core never knows a provider"). A provider
// without an entry still shows in the channel list, without a setup or a
// target picker.
import type { Component } from "svelte";
import type { AppError, ChannelInfo, Verified } from "../lib/api";
import type { Key } from "../lib/i18n.svelte";
import type { PairingView } from "../lib/pairing.svelte";
import WhatsAppSetup from "./whatsapp/Setup.svelte";
import WhatsAppTargetPicker from "./whatsapp/TargetPicker.svelte";
import { targetKind as whatsappTargetKind } from "./whatsapp/target";

export interface SetupProps {
  channel: ChannelInfo;
  view: PairingView | undefined;
  onstart: () => void;
  oncancel: () => void;
  onclose: () => void;
}

/** A target chosen in a picker, checked by the provider where it could be. */
export interface PickedTarget {
  target: unknown;
  display_name: string;
  verified: Verified;
  /** recipient.check_unavailable when it couldn't be checked now. */
  warning: AppError | null;
}

export interface TargetPickerProps {
  channel: ChannelInfo;
  onpick: (p: PickedTarget) => void;
  oncancel: () => void;
}

export interface ProviderUI {
  /** i18n key of the provider's name. */
  label: Key;
  /** The setup panel (pairing), for providers with Capabilities.pairing. */
  setup: Component<SetupProps>;
  /** Chooses a recipient on a channel of this provider. */
  targetPicker: Component<TargetPickerProps>;
  /** The kind of a stored target ("Number", "Group"), or null. */
  targetKind: (target: unknown) => Key | null;
}

export const providerUIs: Record<string, ProviderUI> = {
  whatsapp: {
    label: "provider.whatsapp.name",
    setup: WhatsAppSetup,
    targetPicker: WhatsAppTargetPicker,
    targetKind: whatsappTargetKind,
  },
};

/** The provider's display name, or its type when it has no UI entry. */
export function providerLabel(type: string, t: (k: Key) => string): string {
  const ui = providerUIs[type];
  return ui ? t(ui.label) : type;
}
