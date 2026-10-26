// The stored WhatsApp target: {kind: "number", number} or {kind: "group", jid}
// (internal/provider/whatsapp/target.go).
import type { Key } from "../../lib/i18n.svelte";

export function targetKind(target: unknown): Key | null {
  if (!target || typeof target !== "object" || !("kind" in target)) return null;
  switch (target.kind) {
    case "number":
      return "provider.whatsapp.target.kind.number";
    case "group":
      return "provider.whatsapp.target.kind.group";
    default:
      return null;
  }
}
