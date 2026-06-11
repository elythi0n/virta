import type { ChannelInfo, StreamInfo } from '../daemon/wire.gen';

// One platform's presence for a streamer: its channel key, connection state, and live metadata.
export interface StreamVariant {
  key: string; // "platform:slug"
  platform: string;
  slug: string;
  state: string;
  info?: StreamInfo;
}

// A streamer grouped across the platforms they're joined on. The primary is the variant to surface
// (Twitch preferred, since its thumbnail + viewer count come through where Kick's don't); the rest
// stay reachable from the context menu.
export interface StreamGroup {
  name: string; // lower-cased slug, the grouping key
  display: string; // the primary's slug, for display
  variants: StreamVariant[]; // primary first
  primary: StreamVariant;
  live: boolean; // any variant live
  viewers: number; // combined live viewer count across all variants (shown on the card + sort key)
}

// Platforms with working thumbnails/metadata rank higher, so a streamer on both Twitch and Kick
// surfaces the Twitch card by default.
const PLATFORM_RANK: Record<string, number> = { twitch: 4, kick: 3, youtube: 2, x: 1 };

// score ranks a variant for becoming the group's primary: live first, then having a thumbnail,
// then the platform preference. Higher wins.
function score(v: StreamVariant): number {
  return (v.info?.live ? 1000 : 0) + (v.info?.thumbnail_url ? 100 : 0) + (PLATFORM_RANK[v.platform] ?? 0);
}

// groupStreams collapses channels that share a name (case-insensitive slug) into one streamer card,
// picks each group's primary platform, and sorts live streamers first by viewer count, offline ones
// alphabetically. Channels with the same slug on different platforms are assumed to be the same
// streamer (the common case for simulcasters); distinct people sharing a handle would merge too.
export function groupStreams(channels: ChannelInfo[], streams: Record<string, StreamInfo>): StreamGroup[] {
  const byName = new Map<string, StreamVariant[]>();
  for (const c of channels) {
    const name = c.slug.toLowerCase();
    const key = `${c.platform}:${name}`;
    const variant: StreamVariant = { key, platform: c.platform, slug: c.slug, state: c.state, info: streams[key] };
    const arr = byName.get(name);
    if (arr) arr.push(variant);
    else byName.set(name, [variant]);
  }

  const groups: StreamGroup[] = [];
  for (const [name, variants] of byName) {
    const sorted = [...variants].sort((a, b) => score(b) - score(a));
    const primary = sorted[0];
    const live = variants.some((v) => v.info?.live);
    // Combined audience across every platform the streamer is live on (e.g. Twitch + Kick), so the
    // card reflects their whole reach rather than one source.
    const viewers = variants.reduce((sum, v) => (v.info?.live ? sum + v.info.viewer_count : sum), 0);
    groups.push({ name, display: primary.slug, variants: sorted, primary, live, viewers });
  }

  groups.sort((a, b) => {
    if (a.live !== b.live) return a.live ? -1 : 1; // live first
    if (a.live) return b.viewers - a.viewers || a.name.localeCompare(b.name); // most-viewed on top
    return a.name.localeCompare(b.name); // offline: alphabetical
  });
  return groups;
}
