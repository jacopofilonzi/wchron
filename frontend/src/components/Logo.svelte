<script lang="ts">
  // The wchron logo, inline. Same geometry as frontend/public/logo.svg and
  // internal/logo (keep them in sync, `make icons` regenerates the assets).
  // The fill/stroke colours are brand artwork, not UI colours: that is why they
  // are raw values here instead of the semantic tokens of app.css.
  let { size = 28 }: { size?: number } = $props();
</script>

<svg aria-hidden="true" width={size} height={size} viewBox="0 0 64 64" class="shrink-0">
  <rect width="64" height="64" rx="15" fill="#17171C" />
  <rect x="1" y="1" width="62" height="62" rx="14" fill="none" stroke="#3A3A44" stroke-width="2" />
  <path d="M32 10v6M32 48v6M10 32h6M48 32h6" stroke="#fff" stroke-width="4" stroke-linecap="round" />
  <path d="M32 32l11-11" stroke="#FF8A3D" stroke-width="5" stroke-linecap="round" />
  <circle cx="32" cy="32" r="4.5" fill="#FF8A3D" />
  <rect x="11.5" y="43.5" width="9" height="9" rx="2.5" fill="#FF8A3D" />
</svg>
