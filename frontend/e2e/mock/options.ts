// Options of the fake backend, set by a test before the page loads
// (page.addInitScript), and the log of the calls it received
// (window.__mock.calls, read back with page.evaluate).
export type MockOptions = {
  language?: "it" | "en";
  theme?: "system" | "light" | "dark";
  /** The WhatsApp test channel: connected (default), disconnected, or no channel at all. */
  channels?: "connected" | "disconnected" | "none";
  /** Start with the sample schedules (default) or none. */
  schedules?: "sample" | "none";
};

export type Call = { fn: string; args: unknown[] };

declare global {
  interface Window {
    __mock: { calls: Call[] };
    __MOCK_OPTIONS?: MockOptions;
  }
}

window.__mock = window.__mock ?? { calls: [] };

export function log(fn: string, ...args: unknown[]): void {
  window.__mock.calls.push({ fn, args: JSON.parse(JSON.stringify(args)) });
}

export const options: MockOptions = window.__MOCK_OPTIONS ?? {};
