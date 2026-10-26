// Stand-in for @wailsio/runtime in the end-to-end tests (vite --mode e2e):
// in-memory events, delivered asynchronously like Wails does.
type Listener = (ev: { name: string; data: unknown }) => void;
const listeners = new Map<string, Set<Listener>>();

export const Events = {
  On(name: string, cb: Listener): () => void {
    if (!listeners.has(name)) listeners.set(name, new Set());
    listeners.get(name)!.add(cb);
    return () => listeners.get(name)?.delete(cb);
  },
  Emit(name: string, data?: unknown): void {
    setTimeout(() => listeners.get(name)?.forEach((cb) => cb({ name, data })), 0);
  },
};
