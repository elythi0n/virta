import { describe, expect, it } from 'vitest';
import type { ChannelInfo, StreamInfo } from '../daemon/wire.gen';
import { groupStreams } from './streamGroups';

const ch = (platform: string, slug: string, state = 'connected'): ChannelInfo => ({ platform, slug, state });
const live = (platform: string, slug: string, viewers: number, thumb = ''): Record<string, StreamInfo> => ({
  [`${platform}:${slug.toLowerCase()}`]: {
    platform,
    slug,
    live: true,
    viewer_count: viewers,
    thumbnail_url: thumb,
  },
});

describe('groupStreams', () => {
  it('groups the same streamer across platforms into one card', () => {
    const groups = groupStreams([ch('twitch', 'Forsen'), ch('kick', 'forsen')], {});
    expect(groups).toHaveLength(1);
    expect(groups[0].variants).toHaveLength(2);
    expect(groups[0].name).toBe('forsen');
  });

  it('prefers Twitch as the primary when a streamer is on both platforms', () => {
    const groups = groupStreams([ch('kick', 'forsen'), ch('twitch', 'forsen')], {});
    expect(groups[0].primary.platform).toBe('twitch');
    expect(groups[0].variants[0].platform).toBe('twitch'); // primary first
  });

  it('prefers a live variant with a thumbnail over an offline higher-ranked one', () => {
    // Kick is live with a thumbnail; Twitch is offline. The live one should lead.
    const streams = live('kick', 'xqc', 5000, 'thumb.jpg');
    const groups = groupStreams([ch('twitch', 'xqc'), ch('kick', 'xqc')], streams);
    expect(groups[0].primary.platform).toBe('kick');
    expect(groups[0].live).toBe(true);
  });

  it('sorts live streamers by viewer count, most-viewed first', () => {
    const streams = { ...live('twitch', 'small', 100), ...live('twitch', 'big', 9000) };
    const groups = groupStreams([ch('twitch', 'small'), ch('twitch', 'big')], streams);
    expect(groups.map((g) => g.name)).toEqual(['big', 'small']);
    expect(groups[0].viewers).toBe(9000);
  });

  it('puts live streamers above offline ones, offline sorted alphabetically', () => {
    const streams = live('twitch', 'zed', 50);
    const groups = groupStreams([ch('twitch', 'alice'), ch('twitch', 'zed'), ch('twitch', 'bob')], streams);
    expect(groups.map((g) => g.name)).toEqual(['zed', 'alice', 'bob']);
  });

  it('combines live viewer counts across a group (Twitch + Kick)', () => {
    const streams = { ...live('twitch', 'forsen', 3000), ...live('kick', 'forsen', 800) };
    const groups = groupStreams([ch('twitch', 'forsen'), ch('kick', 'forsen')], streams);
    expect(groups[0].viewers).toBe(3800);
  });

  it('counts only live variants toward the combined total', () => {
    const streams = live('twitch', 'forsen', 3000); // kick offline (no entry)
    const groups = groupStreams([ch('twitch', 'forsen'), ch('kick', 'forsen')], streams);
    expect(groups[0].viewers).toBe(3000);
  });
});
