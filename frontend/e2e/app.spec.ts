import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import type { MockOptions } from "./mock/options";

// Monday 26 October 2026, 09:00 in Rome (the browser's time zone, see the config).
const NOW = new Date("2026-10-26T09:00:00+01:00");
const SHOTS = process.env.E2E_SHOTS;

async function openApp(page: Page, opts: MockOptions = {}, hash = "#/schedules") {
  await page.clock.install({ time: NOW });
  await page.addInitScript((o) => (window.__MOCK_OPTIONS = o), opts);
  await page.goto("/" + hash);
}

const calls = (page: Page) => page.evaluate(() => window.__mock.calls);
const lastCall = async (page: Page, fn: string) => (await calls(page)).filter((c) => c.fn === fn).at(-1);
const nav = (page: Page, name: string) => page.getByRole("navigation", { name: /navigazione|navigation/i }).getByRole("link", { name, exact: true });
async function addNumber(page: Page, number: string) {
  const picker = page.getByRole("group", { name: "Aggiungi destinatario" });
  await picker.getByLabel("Numero con il prefisso internazionale").fill(number);
  await picker.getByRole("button", { name: "Aggiungi", exact: true }).click();
}
// Screenshots for the review (E2E_SHOTS=<dir>): the page scrolls inside <main>, so the window grows instead.
const shot = async (page: Page, name: string) => {
  if (!SHOTS) return;
  const size = page.viewportSize()!;
  const height = await page.evaluate(() => Math.max(document.querySelector("main")!.scrollHeight + 60, window.innerHeight));
  await page.setViewportSize({ width: size.width, height });
  await page.screenshot({ path: `${SHOTS}/${name}.png` });
  await page.setViewportSize(size);
};

test("every Service binding exists in the fake backend", () => {
  const names = (src: string, re: RegExp) => [...src.matchAll(re)].map((m) => m[1]).sort();
  const real = names(readFileSync("bindings/github.com/jacopofilonzi/wchron/internal/app/service.ts", "utf8"), /export function (\w+)\(/g);
  const fake = names(readFileSync("e2e/mock/service.ts", "utf8"), /export const (\w+) =/g);
  expect(fake).toEqual(real);
});

test("navigates the pages", async ({ page }) => {
  await openApp(page);
  await expect(page.getByRole("heading", { level: 1, name: "Pianificazioni" })).toBeVisible();
  await expect(page.getByText("prossimo invio oggi 10:00")).toBeVisible();
  await shot(page, "schedules");
  await nav(page, "Stato e log").click();
  await expect(page.getByRole("heading", { level: 1, name: "Stato e log" })).toBeVisible();
  await nav(page, "Canali").click();
  await expect(page.getByRole("heading", { level: 1, name: "Canali" })).toBeVisible();
  await shot(page, "channels");
  await nav(page, "Impostazioni").click();
  await expect(page.getByRole("heading", { level: 1, name: "Impostazioni" })).toBeVisible();
  expect((await calls(page)).some((c) => c.fn === "AckErrors")).toBe(true);
});

test("empty states guide to Channels first, then to a new schedule", async ({ page }) => {
  await openApp(page, { channels: "none" });
  await expect(page.getByText("Prima aggiungi un canale")).toBeVisible();
  await page.getByRole("link", { name: "Vai a Canali" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Canali" })).toBeVisible();
  await page.goto("about:blank");
  await openApp(page, { schedules: "none" });
  await expect(page.getByText("Ancora nessuna pianificazione")).toBeVisible();
  await page.getByRole("main").getByRole("link", { name: "Nuova pianificazione" }).last().click();
  await expect(page.getByRole("heading", { level: 1, name: "Nuova pianificazione" })).toBeVisible();
});

test("creates a schedule end to end", async ({ page }) => {
  await openApp(page);
  await page.getByRole("link", { name: "Nuova pianificazione" }).click();
  await page.getByLabel("Nome", { exact: true }).fill("Report serale");

  // Every 2 hours from 00:00, only 08:00–18:00, Monday to Friday.
  await page.getByLabel("Ore").fill("2");
  await page.getByLabel("Solo in una fascia oraria").check();
  await page.getByLabel("dalle", { exact: true }).fill("08:00");
  await page.getByLabel("alle", { exact: true }).fill("18:00");
  await page.getByRole("button", { name: "Tutti", exact: true }).click();
  await page.getByRole("button", { name: "Feriali" }).click();
  await expect(page.getByRole("button", { name: "sab" })).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByTestId("next-runs").locator("li").first()).toContainText("10:00");

  // One number recipient, checked on the connected channel.
  await page.getByRole("button", { name: "Aggiungi destinatario" }).click();
  await addNumber(page, "+39 333 1234567");
  const recipients = page.getByRole("list", { name: "A chi" });
  await expect(recipients).toContainText("+393331234567");
  await expect(recipients).toContainText("Verificato");

  // Caption with its live preview.
  await page.getByLabel("Testo inviato con il primo file").fill("Report del {date:DD-MM-YYYY}");
  await expect(page.getByTestId("caption-preview")).toContainText("Report del 26-10-2026");

  // One file with its preview.
  await page.getByRole("button", { name: "+ Aggiungi file" }).click();
  await page.getByLabel("Percorso", { exact: true }).fill("C:\\Report\\listino.pdf");
  await expect(page.getByTestId("file-preview")).toContainText("Adesso: listino.pdf (304 KB)");
  await shot(page, "editor-new");

  await page.getByRole("button", { name: "Salva" }).click();
  await expect(page.getByText("Salvata")).toBeVisible();
  await expect(page).toHaveURL(/#\/schedules\/id/);
  const saved = (await lastCall(page, "SaveSchedule"))!.args[0] as { cadence: { interval: { hours: number }; window: unknown; days: string[] } };
  expect(saved.cadence.interval.hours).toBe(2);
  expect(saved.cadence.window).toEqual({ start: "08:00", end: "18:00" });
  expect(saved.cadence.days).toEqual(["mon", "tue", "wed", "thu", "fri"]);

  await page.getByRole("link", { name: "Torna all'elenco" }).click();
  const row = page.getByRole("row").filter({ hasText: "Report serale" });
  await expect(row).toContainText("Ogni 2h · 08:00–18:00");
  await expect(row).toContainText("lun–ven");
  await expect(row).toContainText("oggi 10:00");
  await expect(row).toContainText("Mai eseguita");
});

test("validation problems show under their fields", async ({ page }) => {
  await openApp(page, {}, "#/schedules/new");
  await page.getByRole("button", { name: "+ Aggiungi file" }).click();
  await page.getByLabel("Percorso", { exact: true }).fill("report.pdf");
  await page.getByLabel("Ore").fill("0");
  await page.getByRole("button", { name: "Salva" }).click();
  await expect(page.getByText(/problemi da correggere/)).toBeVisible();
  await expect(page.getByText("Dai un nome alla pianificazione.")).toBeVisible();
  await expect(page.getByText("Aggiungi almeno un destinatario.")).toBeVisible();
  await expect(page.getByText("L'intervallo deve essere tra 1 minuto e 24 ore.").first()).toBeVisible();
  await expect(page.locator("li").filter({ hasText: "non è un percorso completo" }).first()).toBeVisible();
  await shot(page, "editor-problems");
  expect((await calls(page)).filter((c) => c.fn === "SaveSchedule")).toHaveLength(1);
});

test("reorders files with the arrows", async ({ page }) => {
  await openApp(page, {}, "#/schedules/s1");
  const files = page.getByRole("list", { name: "File, in quest'ordine" });
  await expect(files.getByLabel("Percorso").first()).toHaveValue("C:\\Report\\report_{date:DD-MM-YYYY}.xlsx");
  await files.getByRole("button", { name: "Sposta giù" }).first().click();
  await expect(files.getByLabel("Percorso").first()).toHaveValue("C:\\Report\\listino.pdf");
  await expect(files.getByRole("button", { name: "Sposta su" }).first()).toBeDisabled();
  await page.getByRole("button", { name: "Salva" }).click();
  await expect(page.getByText("Salvata")).toBeVisible();
  const saved = (await lastCall(page, "SaveSchedule"))!.args[0] as { files: { path_pattern: string }[] };
  expect(saved.files.map((f) => f.path_pattern)).toEqual(["C:\\Report\\listino.pdf", "C:\\Report\\report_{date:DD-MM-YYYY}.xlsx"]);
});

test("duplicates and deletes a schedule after the confirmation", async ({ page }) => {
  await openApp(page);
  await page.getByRole("button", { name: "Duplica Report vendite" }).click();
  const copy = page.getByRole("row").filter({ hasText: "Report vendite (copia)" });
  await expect(copy).toBeVisible();
  await expect(copy.getByRole("button", { name: /Attiva/ })).toHaveAttribute("aria-pressed", "false");

  await page.getByRole("button", { name: "Elimina Report vendite (copia)" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Sei sicuro di voler eliminare «Report vendite (copia)»?");
  await dialog.getByRole("button", { name: "Annulla" }).click();
  await expect(copy).toBeVisible();
  await page.getByRole("button", { name: "Elimina Report vendite (copia)" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Elimina" }).click();
  await expect(copy).toHaveCount(0);
  await expect(page.getByRole("row").filter({ hasText: "Report vendite" })).toHaveCount(1);
});

test("the enable toggle saves at once", async ({ page }) => {
  await openApp(page);
  await page.getByRole("button", { name: "Attiva: Chiusura giornaliera" }).click();
  await expect(page.getByRole("button", { name: "Attiva: Chiusura giornaliera" })).toHaveAttribute("aria-pressed", "false");
  expect((await lastCall(page, "SetScheduleEnabled"))!.args).toEqual(["s2", false]);
});

test("asks before leaving the editor with unsaved changes", async ({ page }) => {
  await openApp(page, {}, "#/schedules/s1");
  await page.getByLabel("Nome", { exact: true }).fill("Report vendite nord");
  await expect(page.getByText("Modifiche non salvate").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Invia ora" })).toBeDisabled();
  await nav(page, "Stato e log").click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Uscire senza salvarle?");
  await dialog.getByRole("button", { name: "Annulla" }).click();
  await expect(page.getByLabel("Nome", { exact: true })).toHaveValue("Report vendite nord");
  await expect(page).toHaveURL(/#\/schedules\/s1$/);
  await nav(page, "Stato e log").click();
  await page.getByRole("dialog").getByRole("button", { name: "Esci senza salvare" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Stato e log" })).toBeVisible();
  expect((await calls(page)).some((c) => c.fn === "SaveSchedule")).toBe(false);
});

test("Excel values only: sheet chips with dates and read from the file", async ({ page }) => {
  await openApp(page, {}, "#/schedules/s1");
  const first = page.getByRole("listitem", { name: "File 1" });
  await expect(first.getByLabel("Elaborazione")).toHaveValue("xlsx-snapshot");
  const chips = first.getByRole("list", { name: "Fogli da includere" });
  await expect(chips).toContainText("{date:DD-MM-YYYY}");
  await expect(chips).toContainText("→ 26-10-2026");
  await expect(chips).toContainText("Riepilogo");
  await first.getByRole("button", { name: "Leggi i fogli dal file" }).click();
  const found = first.getByRole("group", { name: "Fogli nel file" });
  await expect(found.getByRole("button", { name: "Riepilogo" })).toHaveAttribute("aria-pressed", "true");
  await expect(found.getByRole("button", { name: "26-10-2026" })).toHaveAttribute("aria-pressed", "true");
  await found.getByRole("button", { name: "Dati" }).click();
  await expect(chips).toContainText("Dati");
  await first.getByRole("button", { name: "Rimuovi Riepilogo" }).click();
  await expect(chips).not.toContainText("Riepilogo");
  await shot(page, "editor-excel");
  await page.getByRole("button", { name: "Salva" }).click();
  await expect(page.getByText("Salvata")).toBeVisible();
  const saved = (await lastCall(page, "SaveSchedule"))!.args[0] as { files: { processor_options: { sheets: string[] } }[] };
  expect(saved.files[0]!.processor_options.sheets).toEqual(["{date:DD-MM-YYYY}", "Dati"]);
  // The second file is a PDF: only "Original" fits it.
  const second = page.getByRole("listitem", { name: "File 2" });
  await expect(second.getByLabel("Elaborazione").locator("option")).toHaveText(["Originale"]);
});

test("send names: preview with numbering when several files match", async ({ page }) => {
  await openApp(page, {}, "#/schedules/s1");
  const second = page.getByRole("listitem", { name: "File 2" });
  await second.getByLabel("Percorso").fill("C:\\Report\\vendite_*.pdf");
  await expect(second.getByTestId("file-preview")).toContainText("2 file corrispondono");
  await second.getByRole("button", { name: "Tutti", exact: true }).click();
  await expect(second.getByTestId("file-preview")).toContainText("vendite_nord.pdf");
  await second.getByLabel("Nome da inviare").fill("Vendite");
  await expect(second.getByTestId("send-name-preview")).toContainText("Vendite (1).pdf, Vendite (2).pdf");
});

test("group search picks a group recipient", async ({ page }) => {
  await openApp(page, {}, "#/schedules/s2");
  await page.getByRole("button", { name: "Aggiungi destinatario" }).click();
  await page.getByRole("button", { name: "Gruppo" }).click();
  await page.getByLabel("Cerca un gruppo").fill("mag");
  const groups = page.getByRole("list", { name: "Gruppi" });
  await expect(groups.getByRole("button")).toHaveText(["Magazzino"]);
  await groups.getByRole("button", { name: "Magazzino" }).click();
  const list = page.getByRole("list", { name: "A chi" });
  await expect(list.getByRole("listitem").filter({ hasText: "Magazzino" })).toContainText("Gruppo");
  await expect(list.getByRole("listitem").filter({ hasText: "Magazzino" })).toContainText("Verificato");
});

test("offline channel: a number is kept unverified, groups need the connection", async ({ page }) => {
  await openApp(page, { channels: "disconnected" }, "#/schedules/s2");
  await page.getByRole("button", { name: "Aggiungi destinatario" }).click();
  await expect(page.getByText(/non è connesso: il numero viene salvato come non verificato/)).toBeVisible();
  await addNumber(page, "+39 347 1112223");
  const item = page.getByRole("list", { name: "A chi" }).getByRole("listitem").filter({ hasText: "+393471112223" });
  await expect(item).toContainText("Non verificato");
  await expect(item).toContainText("Viene controllato da solo");
  await page.getByRole("button", { name: "Aggiungi destinatario" }).click();
  await page.getByRole("button", { name: "Gruppo" }).click();
  await expect(page.getByText("WhatsApp test non è connesso")).toBeVisible();
});

test("removing a saved recipient asks first", async ({ page }) => {
  await openApp(page, {}, "#/schedules/s1");
  await page.getByRole("button", { name: "Rimuovi Vendite Nord" }).click();
  await expect(page.getByRole("dialog")).toContainText("Sei sicuro di voler rimuovere «Vendite Nord»?");
  await page.getByRole("dialog").getByRole("button", { name: "Rimuovi" }).click();
  await expect(page.getByText("Ancora nessun destinatario.")).toBeVisible();
});

test("settings: error recipients, data folder", async ({ page }) => {
  await openApp(page, {}, "#/settings");
  await expect(page.getByText("Nessuno: i problemi arrivano solo")).toBeVisible();
  await page.getByRole("button", { name: "Aggiungi destinatario" }).click();
  await addNumber(page, "+39 347 1234001");
  const list = page.getByRole("list", { name: "Destinatari degli errori" });
  await expect(list).toContainText("+393471234001");
  expect(((await lastCall(page, "SaveErrorRecipients"))!.args[0] as unknown[]).length).toBe(1);
  await expect(page.getByTestId("data-dir")).toContainText("ultimo backup 25 ottobre 2026");
  await page.getByRole("button", { name: "Apri cartella" }).click();
  await expect.poll(() => lastCall(page, "OpenDataDir")).toBeTruthy();
  await expect(page.getByText("Errore imprevisto")).toHaveCount(0);
  await shot(page, "settings");

  await page.getByRole("button", { name: "Rimuovi +393471234001" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Rimuovi" }).click();
  await expect(list).toHaveCount(0);
  expect((await lastCall(page, "SaveErrorRecipients"))!.args[0]).toEqual([]);
});

test("status page: cards, last runs, log filter", async ({ page }) => {
  await openApp(page, {}, "#/status");
  await expect(page.getByTestId("runs-today")).toContainText("2 esecuzioni · 4 file");
  await expect(page.getByTestId("problems-7d")).toContainText("1 con problemi");
  const runs = page.getByRole("list", { name: "Ultime esecuzioni" });
  await expect(runs.getByRole("listitem")).toHaveCount(4);
  await expect(runs.getByRole("listitem").filter({ hasText: "Chiusura giornaliera" })).toContainText("nessun file corrisponde");
  const log = page.getByTestId("log");
  await expect(log.getByRole("listitem")).toHaveCount(5);
  await shot(page, "status");
  await page.getByRole("button", { name: "Errori" }).click();
  await expect(log.getByRole("listitem")).toHaveCount(1);
  await expect(log).toContainText("send failed");
  await page.getByRole("button", { name: "Avvisi" }).click();
  await expect(log.getByRole("listitem")).toHaveCount(2);
  await page.getByRole("button", { name: "Apri cartella" }).click();
  await expect.poll(() => lastCall(page, "OpenLogsDir")).toBeTruthy();
  await expect(page.getByText("Errore imprevisto")).toHaveCount(0);
});

test("dark theme renders with the dark tokens", async ({ page }) => {
  await openApp(page, { theme: "dark" }, "#/schedules/s1");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.locator("body")).toHaveCSS("background-color", "rgb(17, 17, 20)");
  await expect(page.getByRole("heading", { level: 1, name: "Report vendite" })).toBeVisible();
  await shot(page, "editor-dark");
});

test("switches between Italian and English", async ({ page }) => {
  await openApp(page, {}, "#/settings");
  await page.getByLabel("Lingua").selectOption("en");
  await expect(nav(page, "Schedules")).toBeVisible();
  await nav(page, "Schedules").click();
  await expect(page.getByRole("heading", { level: 1, name: "Schedules" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Report vendite" })).toContainText("Every 1h · 08:00–20:00");
  await expect(page.getByRole("row").filter({ hasText: "Report vendite" })).toContainText("Mon–Fri");
  await nav(page, "Settings").click();
  await page.getByLabel("Language").selectOption("it");
  await expect(nav(page, "Pianificazioni")).toBeVisible();
});
