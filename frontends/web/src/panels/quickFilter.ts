import type { FeedMessage } from '@virta/feed-core';

// Quick views over the feed: the important message kinds, surfaced as one-tap chips.
export type QuickFilter = 'all' | 'subs' | 'gifts' | 'raids' | 'mods';

export const QUICK_FILTERS: { value: QuickFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'subs', label: 'Subs' },
  { value: 'gifts', label: 'Gifts' },
  { value: 'raids', label: 'Raids' },
  { value: 'mods', label: 'Mods' },
];

function matchesQuick(m: FeedMessage, f: QuickFilter): boolean {
  switch (f) {
    case 'all':
      return true;
    case 'subs':
      return m.type === 'sub' || m.type === 'resub' || m.type === 'giftsub';
    case 'gifts':
      return m.type === 'giftsub';
    case 'raids':
      return m.type === 'raid' || m.type === 'host';
    case 'mods':
      return !!m.badges?.some((b) => b.set === 'moderator' || b.set === 'broadcaster');
  }
}

// Client-side view filter over the buffered feed: a quick category plus a free-text query (matched
// against author and body). Returns the same array reference when nothing narrows it, so the feed
// skips the extra work and a needless re-render.
export function filterFeed(messages: FeedMessage[], filter: QuickFilter, query: string): FeedMessage[] {
  const q = query.trim().toLowerCase();
  if (filter === 'all' && q === '') return messages;
  return messages.filter(
    (m) => matchesQuick(m, filter) && (q === '' || m.author.toLowerCase().includes(q) || m.body.toLowerCase().includes(q)),
  );
}
