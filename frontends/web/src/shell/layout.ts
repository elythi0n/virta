import type { DockviewApi, SerializedDockview } from 'dockview';

// The dock layout is persisted locally so the workspace survives a restart. (Per-profile,
// daemon-stored layouts — profile `layouts.desktop` — are a later step needing a settings API.)
const KEY = 'virta.layout.v1';

export function loadLayout(): SerializedDockview | null {
  try {
    const raw = localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as SerializedDockview) : null;
  } catch {
    return null;
  }
}

let timer: ReturnType<typeof setTimeout> | null = null;

// Debounced: layout changes fire rapidly during a drag, so coalesce writes.
export function saveLayoutDebounced(api: DockviewApi): void {
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => {
    try {
      localStorage.setItem(KEY, JSON.stringify(api.toJSON()));
    } catch {
      // ignore (private mode / quota)
    }
  }, 500);
}
