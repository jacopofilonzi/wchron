<script lang="ts">
  import { onMount } from "svelte";
  import EmptyPage from "../components/EmptyPage.svelte";
  import { api } from "../lib/api";
  import { t } from "../lib/i18n.svelte";
  import { applySnapshot } from "../lib/state.svelte";

  // Opening the Status page means the user has seen the errors: the tray
  // stops showing the error state (the full page is milestone 8).
  onMount(() => {
    void api.ackErrors().then((r) => {
      if (r.ok) applySnapshot(r.value);
    });
  });
</script>

<EmptyPage title={t("ui.status.title")} text={t("ui.status.empty")} />
