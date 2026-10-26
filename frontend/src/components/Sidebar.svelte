<script lang="ts">
  import Icon from "./Icon.svelte";
  import Logo from "./Logo.svelte";
  import Toggle from "./Toggle.svelte";
  import { api, type AppError } from "../lib/api";
  import { t, tError, type Key } from "../lib/i18n.svelte";
  import { href, pages, route } from "../lib/router.svelte";
  import { app, applySnapshot } from "../lib/state.svelte";

  let error = $state<AppError | null>(null);

  async function setRoutine(on: boolean) {
    const r = await api.setRoutineActive(on);
    error = r.ok ? null : r.error;
    if (r.ok) applySnapshot(r.value);
  }
</script>

<nav aria-label={t("ui.nav.label")} class="flex w-56 shrink-0 flex-col gap-1 border-r border-border bg-sidebar px-3.5 py-5">
  <div class="flex items-center gap-2.5 px-2.5 pt-1 pb-5">
    <Logo size={28} />
    <span class="text-[15px] font-semibold">{t("ui.app.name")}</span>
  </div>

  {#each pages as p (p)}
    <a class="nav-item" href={href(p)} aria-current={route.page === p ? "page" : undefined}>
      <Icon name={p} />{t(`ui.nav.${p}` as Key)}
    </a>
  {/each}

  <div class="grow"></div>

  {#if app.s}
    <div class="card flex flex-col gap-2.5 rounded-[10px] p-3 text-[13px]">
      <div class="flex items-center gap-2">
        <span class="status-dot bg-warning"></span>{t("ui.channels.none")}
      </div>
      <div class="flex items-center justify-between">
        <span>{t("ui.routine.toggle")}</span>
        <Toggle pressed={app.s.routine_active} label={t("ui.routine.toggle")} onchange={setRoutine} />
      </div>
      {#if error}<p role="alert" class="m-0 text-danger">{tError(error.code, error.params)}</p>{/if}
    </div>
  {/if}
</nav>
