import { request } from './http';
import type { Moment, MomentMessage } from './wire.gen';

export type { Moment, MomentMessage };

// Auto-bookmarked chat-spike moments, newest first. `channel` scopes to one "platform:slug";
// `before` pages past a moment id (ids are ULIDs, so they sort by time).
export function listMoments(channel?: string, before?: string, limit?: number): Promise<Moment[]> {
  const params = new URLSearchParams();
  if (channel) params.set('channel', channel);
  if (before) params.set('before', before);
  if (limit) params.set('limit', String(limit));
  const qs = params.toString();
  return request<{ moments: Moment[] }>(`/v1/moments${qs ? `?${qs}` : ''}`).then((r) => r.moments);
}

export function deleteMoment(id: string): Promise<void> {
  return request<void>(`/v1/moments/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
