import { useSyncExternalStore } from 'react';

// Tracks which dock panels have unseen content, as a module-level pub/sub — panel bodies and
// their tabs render in separate dockview trees, so React context can't span the two. A panel
// marks itself when content arrives; its tab shows a dot while backgrounded and clears the mark
// on any activation change, so the dot always means "new since you last looked". Mentions mark
// at a stronger level the tab renders distinctly, and are never downgraded by plain activity.
export type ActivityLevel = 'activity' | 'mention';

const active = new Map<string, ActivityLevel>();
const listeners = new Set<() => void>();
let version = 0;

function emit() {
  version += 1;
  for (const l of listeners) l();
}

/** Mark a panel as having unseen content. No-ops without an id, so call sites stay unconditional. */
export function markActivity(id: string | undefined, level: ActivityLevel = 'activity') {
  if (!id) return;
  const cur = active.get(id);
  if (cur === level || cur === 'mention') return;
  active.set(id, level);
  emit();
}

export function clearActivity(id: string) {
  if (active.delete(id)) emit();
}

export function activityLevel(id: string): ActivityLevel | undefined {
  return active.get(id);
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

/** Subscribes the caller to activity changes; re-renders whenever any panel's state flips. */
export function useActivityVersion(): number {
  return useSyncExternalStore(
    subscribe,
    () => version,
    () => version,
  );
}
