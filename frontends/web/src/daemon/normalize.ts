import type { FeedMessage, Platform, Segment as FeedSegment } from '@virta/feed-core';
import type { EmoteRef, Segment as WireSegment, UnifiedMessage } from './wire.gen';

// Emote CDN templates carry a {size} placeholder the frontend fills; the token differs per
// provider. A medium size reads well in the feed.
const EMOTE_SIZE: Record<string, string> = {
  twitch: '2.0',
  kick: '2',
  '7tv': '2x',
  bttv: '2x',
  ffz: '2x',
};

function emoteUrl(emote: EmoteRef): string {
  const size = EMOTE_SIZE[emote.provider] ?? '2x';
  return emote.url_template.replace('{size}', size);
}

// Map one daemon segment to a feed-core render segment. Cheer and masked carry plain display text,
// so they render as text (masked already shows the mask token; reveal handling comes later).
function toFeedSegment(seg: WireSegment): FeedSegment {
  switch (seg.kind) {
    case 'emote':
      return seg.emote
        ? { type: 'emote', code: seg.emote.name || seg.text, url: emoteUrl(seg.emote) }
        : { type: 'text', text: seg.text };
    case 'mention':
      return { type: 'mention', user: seg.text.replace(/^@/, '') };
    case 'link':
      return { type: 'link', href: seg.text, text: seg.text };
    default:
      return { type: 'text', text: seg.text };
  }
}

function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

// Convert a daemon UnifiedMessage into the feed renderer's message shape.
export function toFeedMessage(m: UnifiedMessage): FeedMessage {
  return {
    id: m.id,
    ts: formatTimestamp(m.sent_at),
    platform: m.platform as Platform,
    author: m.author.display_name || m.author.login,
    authorColor: m.author.color || undefined,
    body: m.segments.map((s) => s.text).join(''),
    segments: m.segments.map(toFeedSegment),
  };
}
