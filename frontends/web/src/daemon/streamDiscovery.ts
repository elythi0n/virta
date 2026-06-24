import { request } from './http';

// Stream-discovery API: search a platform for channels by name, or browse the top live streams.
// Backed by Twitch's helix /search/channels and /streams today; Kick has no public search-channels
// endpoint so it returns an empty list — Discovery uses the AddChannel flow for direct Kick lookup.

export interface DiscoveryChannel {
  platform: string;
  slug: string;
  display_name: string;
  is_live: boolean;
  title?: string;
  category?: string;
  thumbnail?: string;
  tags?: string[];
  started_at?: string;
}

export interface DiscoveryStream {
  platform: string;
  slug: string;
  display_name: string;
  title?: string;
  category?: string;
  viewer_count: number;
  started_at?: string;
  thumbnail?: string;
  language?: string;
  tags?: string[];
}

export interface DiscoveryStreamsPage {
  streams: DiscoveryStream[];
  cursor?: string;
}

export function searchChannels(
  platform: string,
  query: string,
  opts?: { first?: number; liveOnly?: boolean },
): Promise<DiscoveryChannel[]> {
  const q = new URLSearchParams({ platform, q: query });
  if (opts?.first) q.set('first', String(opts.first));
  if (opts?.liveOnly) q.set('live_only', 'true');
  return request<{ results: DiscoveryChannel[] }>(`/v1/discovery/channels?${q.toString()}`).then(
    (r) => r.results,
  );
}

export function topStreams(
  platform: string,
  opts?: { first?: number; cursor?: string; gameId?: string },
): Promise<DiscoveryStreamsPage> {
  const q = new URLSearchParams({ platform });
  if (opts?.first) q.set('first', String(opts.first));
  if (opts?.cursor) q.set('cursor', opts.cursor);
  if (opts?.gameId) q.set('game_id', opts.gameId);
  return request<DiscoveryStreamsPage>(`/v1/discovery/streams?${q.toString()}`);
}
