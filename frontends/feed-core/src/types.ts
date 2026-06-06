export type Platform = 'twitch' | 'kick' | 'x';

// The minimal shape the renderer needs. Mirrors the daemon's UnifiedMessage; body is plain text
// for now and becomes a segment list (text/emote/mention) when the segment renderer lands.
export interface FeedMessage {
  /** Stable key; a ULID in production. */
  id: string;
  /** Preformatted timestamp for display. */
  ts: string;
  platform: Platform;
  author: string;
  /** Platform-provided author color; contrast-clamped before use (later). */
  authorColor?: string;
  body: string;
}
