// Shared types for the Studio: VOD review, the configurable chat overlay, clips, and presets.

export type SourceKind = 'twitch' | 'local';

// A single replayed chat line, normalized across sources (Twitch VOD comments today; logged chat
// later). `offset` is seconds from the start of the recording.
export interface ChatLine {
  id: string;
  offset: number;
  author: string;
  color: string; // resolved, contrast-clamped hex
  text: string;
}

// The chat overlay's free-form rectangle over the video, as fractions of the stage (0–1), so it
// stays correct at any resolution. The user drags to move it and pulls the corners to resize.
export interface OverlayRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export type OverlayTheme = 'glass' | 'solid' | 'shadow' | 'outline' | 'minimal';
export type OverlayFont = 'ui' | 'mono' | 'rounded' | 'condensed';
export type NameStyle = 'colored' | 'mono' | 'bold' | 'hidden';

// Everything about how chat is composited into a clip — the heart of the "highly configurable" ask.
export interface OverlayConfig {
  enabled: boolean;
  rect: OverlayRect; // free position + size, fractions of the stage
  scale: number; // font scale, 0.6–2.0
  opacity: number; // 0–1 background opacity
  theme: OverlayTheme;
  font: OverlayFont;
  nameStyle: NameStyle;
  accent: string; // accent / default name color (hex)
  maxLines: number; // most lines shown at once
  fadeMs: number; // per-line fade-in duration
  lineHold: number; // seconds a line stays before fading out (0 = never)
  showTimestamps: boolean;
  shadow: boolean; // drop shadow on text for legibility over busy video
  padding: number; // inner padding scale
}

// A user-saved look + layout. "Auto mode" applies the active preset to every new clip.
export interface StudioPreset {
  id: string;
  name: string;
  overlay: OverlayConfig;
}

// A defined clip: an in/out range over a source plus the overlay look to bake in.
export interface ClipSpec {
  id: string;
  title: string;
  source: SourceKind;
  sourceRef: string; // twitch vod id, or local file name
  channel: string; // "platform:slug" when known, for logged-chat lookups
  inSec: number;
  outSec: number;
  createdAtMs: number;
  overlay: OverlayConfig;
  presetName?: string;
}

// A detected hype moment placed on the timeline.
export interface TimelineMoment {
  id: string;
  startSec: number;
  endSec: number;
  peakRate: number;
  baseline: number;
}

export const DEFAULT_OVERLAY: OverlayConfig = {
  enabled: true,
  rect: { x: 0.03, y: 0.42, w: 0.3, h: 0.54 },
  scale: 1,
  opacity: 0.55,
  theme: 'glass',
  font: 'ui',
  nameStyle: 'colored',
  accent: '#5B8CFF',
  maxLines: 8,
  fadeMs: 180,
  lineHold: 0,
  showTimestamps: false,
  shadow: true,
  padding: 1,
};
