<script lang="ts">
  // A QR code drawn as SVG. The encoder (uqr) is loaded lazily, only when a
  // pairing shows a code. Dark modules on a light ground in every theme, with
  // a quiet zone, so phones can read it.
  type Encoded = { size: number; data: boolean[][] };
  let { text, label, size = 232 }: { text: string; label: string; size?: number } = $props();

  let encoded = $state<Encoded | null>(null);
  let failed = $state(false);

  $effect(() => {
    const value = text;
    let stale = false;
    import("uqr")
      .then(({ encode }) => {
        if (!stale) encoded = encode(value, { ecc: "L", border: 3 });
      })
      .catch(() => {
        if (!stale) failed = true;
      });
    return () => {
      stale = true;
    };
  });

  const path = $derived.by(() => {
    if (!encoded) return "";
    let d = "";
    encoded.data.forEach((row, y) =>
      row.forEach((dark, x) => {
        if (dark) d += `M${x} ${y}h1v1h-1z`;
      }),
    );
    return d;
  });
</script>

{#if encoded}
  <svg
    role="img"
    aria-label={label}
    width={size}
    height={size}
    viewBox="0 0 {encoded.size} {encoded.size}"
    shape-rendering="crispEdges"
    class="block rounded-lg bg-qr-light"
  >
    <path d={path} class="fill-qr-dark" />
  </svg>
{:else}
  <div
    class="flex items-center justify-center rounded-lg bg-qr-light text-[13px] text-muted"
    style="width: {size}px; height: {size}px"
    aria-busy={!failed}
  >
    {#if failed}<span class="text-danger">!</span>{/if}
  </div>
{/if}
