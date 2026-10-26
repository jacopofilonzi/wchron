<script lang="ts">
  import Toggle from "../components/Toggle.svelte";
  import { api, Langs, Themes, type AppError, type Lang, type Snapshot, type Theme } from "../lib/api";
  import { t, tError, type Key } from "../lib/i18n.svelte";
  import { app, applySnapshot } from "../lib/state.svelte";

  type Row = "appearance" | "autostart" | "keepawake" | "keepawake_default";
  let errors = $state<Partial<Record<Row, AppError>>>({});

  const themes: { value: Theme; key: Key }[] = [
    { value: Themes.ThemeSystem, key: "ui.settings.theme.system" },
    { value: Themes.ThemeLight, key: "ui.settings.theme.light" },
    { value: Themes.ThemeDark, key: "ui.settings.theme.dark" },
  ];
  const langs: { value: Lang; key: Key }[] = [
    { value: Langs.IT, key: "ui.settings.language.it" },
    { value: Langs.EN, key: "ui.settings.language.en" },
  ];

  async function run(row: Row, p: Promise<{ ok: true; value: Snapshot } | { ok: false; error: AppError }>) {
    const r = await p;
    if (r.ok) {
      delete errors[row];
      applySnapshot(r.value);
    } else {
      errors[row] = r.error;
    }
  }
</script>

{#snippet error(row: Row)}
  {#if errors[row]}
    <p role="alert" class="m-0 mt-1 text-danger">{tError(errors[row].code, errors[row].params)}</p>
  {/if}
{/snippet}

<h1 class="page-title">{t("ui.settings.title")}</h1>

{#if app.s}
  {@const s = app.s}
  <section class="card text-[13px]" aria-label={t("ui.settings.appearance")}>
    <div class="setting-row">
      <label for="theme" class="grow text-sm font-medium">{t("ui.settings.theme")}</label>
      <select
        id="theme"
        class="field"
        value={s.theme}
        onchange={(e) => run("appearance", api.setTheme(e.currentTarget.value as Theme))}
      >
        {#each themes as th (th.value)}<option value={th.value}>{t(th.key)}</option>{/each}
      </select>
    </div>
    <div class="setting-row">
      <label for="language" class="grow text-sm font-medium">{t("ui.settings.language")}</label>
      <select
        id="language"
        class="field"
        value={s.language}
        onchange={(e) => run("appearance", api.setLanguage(e.currentTarget.value as Lang))}
      >
        {#each langs as l (l.value)}<option value={l.value}>{t(l.key)}</option>{/each}
      </select>
    </div>
    {#if errors.appearance}<div class="px-[18px] pb-3">{@render error("appearance")}</div>{/if}
  </section>

  <section class="card text-[13px]" aria-label={t("ui.settings.behaviour")}>
    <div class="setting-row">
      <div class="grow">
        <div class="text-sm font-medium">{t("ui.settings.autostart")}</div>
        <div class="mt-0.5 text-muted">{t("ui.settings.autostart.hint")}</div>
        {@render error("autostart")}
      </div>
      <Toggle pressed={s.autostart} label={t("ui.settings.autostart")} onchange={(on) => run("autostart", api.setAutostart(on))} />
    </div>
    <div class="setting-row">
      <div class="grow">
        <div class="text-sm font-medium">{t("ui.settings.keepawake")}</div>
        <div class="mt-0.5 text-muted">{t("ui.settings.keepawake.hint")}</div>
        {@render error("keepawake")}
      </div>
      <Toggle pressed={s.keep_awake} label={t("ui.settings.keepawake")} onchange={(on) => run("keepawake", api.setKeepAwake(on))} />
    </div>
    <div class="setting-row">
      <div class="grow">
        <div class="text-sm font-medium">{t("ui.settings.keepawake_default")}</div>
        <div class="mt-0.5 text-muted">{t("ui.settings.keepawake_default.hint")}</div>
        {@render error("keepawake_default")}
      </div>
      <Toggle
        pressed={s.keep_awake_default}
        label={t("ui.settings.keepawake_default")}
        onchange={(on) => run("keepawake_default", api.setKeepAwakeDefault(on))}
      />
    </div>
    <div class="setting-row">
      <div class="grow text-sm font-medium">{t("ui.settings.version", { version: s.version })}</div>
    </div>
  </section>
{/if}
