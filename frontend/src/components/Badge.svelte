<script lang="ts">
  // A small coloured label.
  let { tone = "neutral", text }: { tone?: "neutral" | "success" | "warning" | "danger" | "accent"; text: string } = $props();

  const tones = {
    neutral: "bg-segment text-text-soft",
    success: "bg-success-soft text-success",
    warning: "bg-warning-soft text-warning",
    danger: "bg-danger-soft text-danger",
    accent: "bg-accent-soft text-on-accent-soft",
  } as const;
</script>

<span class="badge {tones[tone]}">{text}</span>
