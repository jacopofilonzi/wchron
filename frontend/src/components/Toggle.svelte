<script lang="ts">
  // A switch: a real <button aria-pressed>, labelled for screen readers.
  let { pressed, label, disabled = false, onchange }: {
    pressed: boolean;
    label: string;
    disabled?: boolean;
    onchange: (next: boolean) => void;
  } = $props();
</script>

<button
  type="button"
  aria-pressed={pressed}
  aria-label={label}
  {disabled}
  onclick={() => onchange(!pressed)}
  class="flex h-5 w-[34px] shrink-0 cursor-pointer rounded-full border-0 p-0.5 transition-colors disabled:cursor-default disabled:opacity-50
    {pressed ? 'justify-end bg-accent' : 'justify-start bg-toggle-off'}"
>
  <span class="block size-4 rounded-full bg-knob shadow-sm"></span>
</button>
