import type { DockviewApi, SerializedDockview } from 'dockview';

// Deck workspace layout: persisted separately from the main dock so resetting one doesn't touch
// the other. The structure is dockview's own serialized shape, so we can change the default seed
// later without losing a user's rearrangements.
// Bump on seed-shape changes (added panels, renamed kinds) so existing users get the new
// defaults instead of an old layout that's missing the new panes.
const KEY = 'virta.deck.layout.v4';

export function loadDeckLayout(): SerializedDockview | null {
  try {
    const raw = localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as SerializedDockview) : null;
  } catch {
    return null;
  }
}

let timer: ReturnType<typeof setTimeout> | null = null;

export function saveDeckLayoutDebounced(api: DockviewApi): void {
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => {
    try {
      localStorage.setItem(KEY, JSON.stringify(api.toJSON()));
    } catch {
      // ignore (private mode / quota)
    }
  }, 500);
}
