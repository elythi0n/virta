import type { FeedMessage } from '@virta/feed-core';

const collapsible = (m: FeedMessage) => (!m.type || m.type === 'chat' || m.type === 'action') && !m.deleted;

// A message whose visible content is only emotes (emote segments plus optional whitespace) — the
// "emote wall" of a hype moment. Calm mode folds a run of these together even when the emotes
// differ, since the wall reads as one beat regardless of which emotes scroll by.
const emoteOnly = (m: FeedMessage) =>
  m.segments.length > 0 &&
  m.segments.some((s) => s.type === 'emote') &&
  m.segments.every((s) => s.type === 'emote' || (s.type === 'text' && s.text.trim() === ''));

// Two adjacent messages fold into one combo row when they're identical chat, or both emote-only
// (an emote wall). Same author isn't required — overlapping spam from many chatters is the case
// calm mode most needs to tame.
function foldable(a: FeedMessage, b: FeedMessage): boolean {
  if (!collapsible(a) || !collapsible(b)) return false;
  if (a.body === b.body && b.body.trim() !== '') return true;
  return emoteOnly(a) && emoteOnly(b);
}

// Calm mode: fold runs of consecutive chat messages into one row carrying a combo count — identical
// text (a spammed "LUL" ×40) or an emote wall — so each reads as a single line. Display-only: the
// buffer (and the daemon's logger/webhook sinks) keep every message; this only changes what's
// drawn. The run's first message stays as the representative (stable key) and its count climbs.
export function collapseCombos(messages: FeedMessage[]): FeedMessage[] {
  const out: FeedMessage[] = [];
  for (const m of messages) {
    const last = out[out.length - 1];
    if (last && foldable(last, m)) {
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
