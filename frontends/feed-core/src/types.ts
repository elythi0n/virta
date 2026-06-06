import type { Segment } from './segments';

export type Platform = 'twitch' | 'kick' | 'x';

export type MessageType =
  | 'chat'
  | 'action'
  | 'reply'
  | 'sub'
  | 'resub'
  | 'giftsub'
  | 'raid'
  | 'host'
  | 'follow'
  | 'announcement'
  | 'moderation'
  | 'system';

// The minimal shape the renderer needs. Mirrors the daemon's UnifiedMessage. `body` is the raw
// text (for copy/accessibility); `segments` is the parsed, render-ready form.
export interface FeedMessage {
  /** Stable key; a ULID in production. */
  id: string;
  /** Preformatted timestamp for display. */
  ts: string;
  platform: Platform;
  /** Message type; non-chat types (sub, raid, …) render as event bands. Defaults to chat. */
  type?: MessageType;
  /** Platform's own message id, for matching deletions that aren't resolved to our id. */
  platformMessageId?: string;
  /** Author's platform id, for matching channel clears/timeouts to a user. */
  authorId?: string;
  /** Set when a moderator deletes this message (or a clear/timeout removes it). */
  deleted?: boolean;
  author: string;
  /** Platform-provided author color (hex); contrast-clamped before use. */
  authorColor?: string;
  /** Source channel, shown as an attribution tag when a feed aggregates several channels. */
  source?: { slug: string; label: string };
  /** Author badges (broadcaster, moderator, subscriber, …); up to 3 shown, then "+N". `url` is
   *  the resolved artwork; without it the row shows a text chip. */
  badges?: { set: string; title?: string; url?: string }[];
  /** Magnitude for event-type rows: gift-sub count, raid/host viewer count, resub months, sub
   *  tier. Drives the tiered event band and the live celebration banner; absent → ordinary band. */
  event?: { count?: number; viewers?: number; months?: number; tier?: string };
  body: string;
  segments: Segment[];
}
