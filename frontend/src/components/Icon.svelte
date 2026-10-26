<script lang="ts">
  // Inline stroke icons (24×24 grid), so no icon library is needed: the nav
  // pages and a few actions.
  import type { Page } from "../lib/router.svelte";

  let { name, size = 16 }: { name: Page | "copy" | "trash" | "up" | "down" | "close" | "folder"; size?: number } = $props();
</script>

<svg aria-hidden="true" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  {#if name === "schedules"}
    <circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" />
  {:else if name === "status"}
    <path d="M4 19V5M4 19h16M8 15l4-4 3 3 5-6" />
  {:else if name === "channels"}
    <path d="M21 12a8 8 0 0 1-11.8 7L4 20l1-5A8 8 0 1 1 21 12z" />
  {:else if name === "settings"}
    <circle cx="12" cy="12" r="3" /><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M5 19l2-2M17 7l2-2" />
  {:else if name === "copy"}
    <rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15V6a2 2 0 0 1 2-2h8" />
  {:else if name === "trash"}
    <path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" />
  {:else if name === "up"}
    <path d="M12 19V5M6 11l6-6 6 6" />
  {:else if name === "down"}
    <path d="M12 5v14M6 13l6 6 6-6" />
  {:else if name === "close"}
    <path d="M6 6l12 12M18 6L6 18" />
  {:else}
    <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
  {/if}
</svg>
