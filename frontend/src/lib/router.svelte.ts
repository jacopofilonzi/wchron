// A minimal hash router: the five pages need no library. Unknown hashes fall
// back to the first page.
export const pages = ["schedules", "status", "channels", "settings"] as const;
export type Page = (typeof pages)[number];

function parse(hash: string): Page {
  const name = hash.replace(/^#\/?/, "");
  return (pages as readonly string[]).includes(name) ? (name as Page) : "schedules";
}

export const route = $state<{ page: Page }>({ page: parse(location.hash) });

export function startRouter(): () => void {
  const onHash = () => {
    route.page = parse(location.hash);
  };
  window.addEventListener("hashchange", onHash);
  return () => window.removeEventListener("hashchange", onHash);
}

export function href(p: Page): string {
  return `#/${p}`;
}
