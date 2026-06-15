import { request } from './http';
import type { DeckCategory, DeckChannelState, DeckInfo, DeckResult } from './wire.gen';

export type { DeckCategory, DeckChannelState, DeckInfo, DeckResult };

// Status values the daemon emits for each per-channel sync result.
export type DeckStatus = 'ok' | 'error' | 'excluded' | 'unsupported';

// Read the currently-set stream metadata for the given channels, so the Deck form can pre-fill
// with what the streamer already has live.
export function getDeckChannelInfo(channels: string[]): Promise<DeckChannelState[]> {
  const qs = new URLSearchParams({ channels: channels.join(',') }).toString();
  return request<{ channels: DeckChannelState[] }>(`/v1/deck/channel-info?${qs}`).then((r) => r.channels);
}

// Push title/category/tags to every channel in one go; the daemon fans out per platform and
// returns each target's disposition.
export function updateDeckChannelInfo(channels: string[], info: DeckInfo): Promise<DeckResult[]> {
  return request<{ results: DeckResult[] }>('/v1/deck/channel-info', {
    method: 'POST',
    body: JSON.stringify({ channels, info }),
  }).then((r) => r.results);
}

// Search a platform's categories. Used to resolve a free-text game name before sending the patch.
export function searchDeckCategories(platform: string, q: string): Promise<DeckCategory[]> {
  const qs = new URLSearchParams({ platform, q }).toString();
  return request<{ categories: DeckCategory[] }>(`/v1/deck/categories?${qs}`).then((r) => r.categories);
}
