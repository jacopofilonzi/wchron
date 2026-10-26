<script lang="ts">
  import Sidebar from "./components/Sidebar.svelte";
  import Schedules from "./pages/Schedules.svelte";
  import Settings from "./pages/Settings.svelte";
  import Status from "./pages/Status.svelte";
  import Channels from "./pages/Channels.svelte";
  import { route, startRouter } from "./lib/router.svelte";
  import { startState } from "./lib/state.svelte";

  const views = { schedules: Schedules, status: Status, channels: Channels, settings: Settings };
  const View = $derived(views[route.page]);

  $effect(() => startRouter());
  $effect(() => startState());
</script>

<div class="flex h-full bg-ground text-text">
  <Sidebar />
  <main class="flex min-w-0 grow flex-col gap-4 overflow-y-auto px-8 py-7">
    <View />
  </main>
</div>
