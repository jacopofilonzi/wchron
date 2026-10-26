<script lang="ts">
  // Settings: error recipients (saved at once; removing asks first),
  // appearance, behaviour, data folder and last backup, version.
  import ConfirmDialog from "../components/ConfirmDialog.svelte";
  import RecipientList from "../components/RecipientList.svelte";
  import RecipientPicker from "../components/RecipientPicker.svelte";
  import Toggle from "../components/Toggle.svelte";
  import { api, Langs, Themes, type AppError, type Lang, type Recipient, type Snapshot, type Theme } from "../lib/api";
  import { longDate } from "../lib/format";
  import { t, tError, type Key } from "../lib/i18n.svelte";
  import { app, applySnapshot } from "../lib/state.svelte";

  let recipients = $state<Recipient[] | null>(null);
  let recipientsError = $state<AppError | null>(null);
  let recipientProblems = $state<Record<string, AppError>>({});
  let adding = $state(false);
  let removing = $state<Recipient | null>(null);
  let dataDir = $state("");
  let lastBackup = $state<string | null>(null);
  let folderError = $state<AppError | null>(null);

  $effect(() => {
    void api.errorRecipients().then((r) => {
      if (r.ok) recipients = r.value;
      else recipientsError = r.error;
    });
    void api.dataDir().then((r) => {
      if (r.ok) dataDir = r.value;
    });
    void api.lastBackup().then((r) => {
      if (r.ok) lastBackup = r.value;
    });
  });

  async function saveRecipients(next: Recipient[]): Promise<boolean> {
    const r = await api.saveErrorRecipients(next);
    if (!r.ok) {
      recipientsError = r.error;
      return false;
    }
    recipientsError = null;
    recipientProblems = Object.fromEntries(r.value.map((p) => [p.field.replace(/^error_recipients\./, ""), p.error]));
    if (r.value.length > 0) return false;
    recipients = next;
    return true;
  }

  async function addRecipient(rc: Recipient) {
    if (await saveRecipients([...(recipients ?? []), rc])) adding = false;
  }

  async function removeRecipient() {
    if (!removing) return;
    const id = removing.id;
    removing = null;
    await saveRecipients((recipients ?? []).filter((x) => x.id !== id));
  }

  async function openData() {
    const r = await api.openDataDir();
    folderError = r.ok ? null : r.error;
  }

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
  <section class="section-card" aria-labelledby="errrcp-title">
    <div class="flex items-center justify-between gap-4">
      <div>
        <h2 id="errrcp-title" class="section-title">{t("ui.settings.error_recipients")}</h2>
        <p class="m-0 mt-0.5 text-muted">{t("ui.settings.error_recipients.hint")}</p>
      </div>
      {#if !adding}<button type="button" class="btn" onclick={() => (adding = true)}>{t("ui.recipients.add")}</button>{/if}
    </div>
    {#if recipients}
      <RecipientList
        {recipients}
        channels={s.channel_list ?? []}
        label={t("ui.settings.error_recipients")}
        errors={recipientProblems}
        onremove={(r) => (removing = r)}
      />
      {#if recipients.length === 0 && !adding}<p class="m-0 text-muted">{t("ui.settings.error_recipients.empty")}</p>{/if}
    {/if}
    {#if adding}<RecipientPicker channels={s.channel_list ?? []} onadd={addRecipient} oncancel={() => (adding = false)} />{/if}
    {#if recipientsError}<p role="alert" class="m-0 text-danger">{tError(recipientsError.code, recipientsError.params)}</p>{/if}
  </section>

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
  </section>

  <section class="card text-[13px]" aria-label={t("ui.settings.data")}>
    <div class="setting-row">
      <div class="min-w-0 grow">
        <div class="text-sm font-medium">{t("ui.settings.data")}</div>
        <div class="mt-0.5 truncate text-muted select-text" data-testid="data-dir">
          {dataDir}{lastBackup ? ` · ${t("ui.settings.data.last_backup", { date: longDate(lastBackup) })}` : ` · ${t("ui.settings.data.no_backup")}`}
        </div>
        {#if folderError}<p role="alert" class="m-0 mt-1 text-danger">{tError(folderError.code, folderError.params)}</p>{/if}
      </div>
      <button type="button" class="btn" onclick={openData}>{t("ui.settings.data.open")}</button>
    </div>
    <div class="setting-row">
      <div class="grow text-sm font-medium">{t("ui.settings.version", { version: s.version })}</div>
    </div>
  </section>
{/if}

{#if removing}
  <ConfirmDialog title={t("ui.recipients.remove.title")} confirmLabel={t("ui.recipients.remove")} onconfirm={removeRecipient} oncancel={() => (removing = null)}>
    <p class="m-0 text-sm">{t("ui.recipients.remove.confirm", { name: removing.display_name })}</p>
    <p class="m-0 text-text-soft">{t("ui.settings.error_recipients.remove_consequences")}</p>
  </ConfirmDialog>
{/if}
