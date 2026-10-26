<script lang="ts">
  // An "HH:MM" text input (24 h, whatever the Windows locale): the backend
  // checks the value, the pattern only helps the keyboard.
  let {
    id,
    value = $bindable(),
    label = undefined,
    invalid = false,
    class: cls = "",
  }: { id?: string; value: string; label?: string; invalid?: boolean; class?: string } = $props();
</script>

<input
  {id}
  class="input w-[88px] tabular-nums {cls}"
  aria-label={label}
  aria-invalid={invalid}
  inputmode="numeric"
  maxlength="5"
  placeholder="HH:MM"
  bind:value
/>
