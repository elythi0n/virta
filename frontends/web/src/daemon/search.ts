import { request } from './http';
import type { LoggedMessage } from './wire.gen';

export interface SearchParams {
  q: string;
  channel?: string; // "platform:slug"; omit for every logged channel
  author?: string;
  before?: string; // ULID cursor for paging
  limit?: number;
}

// Full-text search over the logged-message store. Returns [] when logging is off (nothing logged)
// or nothing matches.
export function searchMessages(p: SearchParams): Promise<LoggedMessage[]> {
  const qs = new URLSearchParams({ q: p.q });
  if (p.channel) qs.set('channel', p.channel);
  if (p.author) qs.set('author', p.author);
  if (p.before) qs.set('before', p.before);
  if (p.limit) qs.set('limit', String(p.limit));
  return request<{ messages: LoggedMessage[] }>(`/v1/search?${qs.toString()}`).then((r) => r.messages);
}

// Per-channel scrollback, newest-first, from the log.
export function getHistory(channel: string, before = '', limit = 100): Promise<LoggedMessage[]> {
  const qs = new URLSearchParams({ channel, limit: String(limit) });
  if (before) qs.set('before', before);
  return request<{ messages: LoggedMessage[] }>(`/v1/history?${qs.toString()}`).then((r) => r.messages);
}
