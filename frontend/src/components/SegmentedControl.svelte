<script lang="ts" generics="T extends string">
  // A segmented control: one pressed button among a few options.
  let {
    label,
    options,
    value,
    onchange,
  }: {
    label: string;
    options: { value: T; label: string }[];
    value: T;
    onchange: (v: T) => void;
  } = $props();
</script>

<div class="segmented" role="group" aria-label={label}>
  {#each options as o (o.value)}
    <button type="button" aria-pressed={o.value === value} onclick={() => onchange(o.value)}>{o.label}</button>
  {/each}
</div>
