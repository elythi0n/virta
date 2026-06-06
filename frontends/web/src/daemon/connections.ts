import { request } from './http';

// Pinned per-platform connection methods ("platform" → "automatic"|"anonymous"|"authenticated").
export function listMethods(): Promise<Record<string, string>> {
  return request<{ methods: Record<string, string> }>('/v1/connections/methods').then((r) => r.methods);
}

// Pin a platform's connection method; the daemon reconnects that platform's channels. Returns the
// updated method map.
export function setMethod(platform: string, method: string): Promise<Record<string, string>> {
  return request<{ methods: Record<string, string> }>('/v1/connections/method', {
    method: 'PUT',
    body: JSON.stringify({ platform, method }),
  }).then((r) => r.methods);
}
