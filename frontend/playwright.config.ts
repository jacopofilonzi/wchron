import { defineConfig, devices } from "@playwright/test";

// End-to-end tests of the UI (make e2e): Vite in e2e mode (fake backend, see
// e2e/mock: no Wails, no disk, nothing sent) + Chromium. PW_CHANNEL=msedge or
// chrome uses the system browser when Playwright's own isn't installed.
const channel = process.env.PW_CHANNEL || undefined;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  reporter: [["list"]],
  timeout: 30_000,
  use: {
    baseURL: "http://127.0.0.1:9246",
    viewport: { width: 1120, height: 800 },
    trace: "retain-on-failure",
    locale: "it-IT",
    timezoneId: "Europe/Rome",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1120, height: 800 }, channel } }],
  webServer: {
    command: "npx vite --mode e2e",
    url: "http://127.0.0.1:9246",
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
});
