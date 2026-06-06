import type { FeedMessage } from '@virta/feed-core';

const collapsible = (m: FeedMessage) => (!m.type || m.type === 'chat' || m.type === 'action') && !m.deleted;

// Calm mode: fold runs of consecutive chat messages with identical text into one row carrying a
// combo count, so a spammed "LUL" ×40 reads as a single line. Display-only — the buffer (and the
// daemon's logger/webhook sinks) keep every message; this only changes what's drawn. The run's
// first message stays as the representative (stable key) and its combo count climbs.
export function collapseCombos(messages: FeedMessage[]): FeedMessage[] {
  const out: FeedMessage[] = [];
  for (const m of messages) {
    const last = out[out.length - 1];
    if (last && collapsible(last) && collapsible(m) && last.body === m.body && m.body.trim() !== '') {
      out[out.length - 1] = { ...last, combo: (last.combo ?? 1) + 1 };
    } else {
      out.push(m);
    }
  }
  return out;
}

// Calm mode for a flooded channel: drop the messages the daemon's velocity stage marked as
// sampled, then collapse the remaining repeats. The daemon keeps priority lanes (mods, subs,
// first-timers, events) unmarked, so those always survive. Display-only, like collapseCombos:
// the buffer and the daemon's sinks still hold every message. Returns the thinned view plus how
// many sampled rows were hidden, so the UI can show a count.
export function applyCalm(messages: FeedMessage[]): { visible: FeedMessage[]; thinned: number } {
  let thinned = 0;
  const kept: FeedMessage[] = [];
  for (const m of messages) {
    if (m.sampled) {
      thinned++;
      continue;
    }
    kept.push(m);
  }
  return { visible: collapseCombos(kept), thinned };
}
