// The pairing steps pushed by Go (channel:qr), per channel: the QR code to
// show and how the last pairing ended. The channels themselves come with the
// Snapshot (app.s.channel_list).
import { fromRef, onPairing, PairKind, type AppError } from "./api";

export interface PairingView {
  /** The QR contents to show; "" until the first code arrives. */
  code: string;
  /** How the pairing ended; null while it runs. */
  result: PairKind | null;
  error: AppError | null;
}

export const pairings = $state<Record<string, PairingView>>({});

/** Marks a pairing as started (before the first code arrives). */
export function pairingStarted(id: string): void {
  pairings[id] = { code: "", result: null, error: null };
}

/** Forgets a channel's pairing view (closed panel, deleted channel). */
export function clearPairing(id: string): void {
  delete pairings[id];
}

/** Follows the pairing events; returns the cleanup function. */
export function startPairings(): () => void {
  return onPairing((p) => {
    const cur = pairings[p.channel_id] ?? { code: "", result: null, error: null };
    if (p.kind === PairKind.PairCode) {
      pairings[p.channel_id] = { code: p.code, result: null, error: null };
    } else {
      pairings[p.channel_id] = { ...cur, code: "", result: p.kind, error: fromRef(p.error) };
    }
  });
}
