<script lang="ts">
  // Placeholder page: title and an empty-state card, until the real screen lands.
  let { title, text }: { title: string; text: string } = $props();
</script>

<h1 class="page-title">{title}</h1>
<div class="card px-6 py-10 text-center text-[13px] text-muted">{text}</div>
