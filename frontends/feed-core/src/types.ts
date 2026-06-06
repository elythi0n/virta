import type { Segment } from './segments';

export type Platform = 'twitch' | 'kick' | 'x';

// The minimal shape the renderer needs. Mirrors the daemon's UnifiedMessage. `body` is the raw
// text (for copy/accessibility); `segments` is the parsed, render-ready form.
export interface FeedMessage {
  /** Stable key; a ULID in production. */
  id: string;
  /** Preformatted timestamp for display. */
  ts: string;
  platform: Platform;
  author: string;
  /** Platform-provided author color (hex); contrast-clamped before use. */
  authorColor?: string;
  /** Source channel, shown as an attribution tag when a feed aggregates several channels. */
  source?: { slug: string; label: string };
  /** Author badges (broadcaster, moderator, subscriber, …); up to 3 shown, then "+N". */
  badges?: { set: string; title?: string }[];
  body: string;
  segments: Segment[];
}
