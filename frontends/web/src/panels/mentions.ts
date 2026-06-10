import type { FeedMessage } from '@virta/feed-core';

// A message mentions you when one of your configured names appears in its text. Names must be
// pre-normalized (lowercased, trimmed, empties dropped); the body is lowercased here per call.
export function mentionsMe(m: FeedMessage, names: string[]): boolean {
  if (names.length === 0) return false;
  const body = m.body.toLowerCase();
  return names.some((n) => body.includes(n));
}
