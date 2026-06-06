import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { joinChannel, leaveChannel, listChannels } from './api';
import { resetDiscovery } from './discovery';

const DISCOVERY = { addr: '127.0.0.1:9999', token: 'tok-xyz' };

type Call = { url: string; init?: RequestInit };

// Mock fetch: answer /__discovery, and let each test supply the API response.
function mockFetch(apiResponse: () => Response): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, init });
    if (url.endsWith('/__discovery')) {
      return Promise.resolve(new Response(JSON.stringify(DISCOVERY), { status: 200 }));
    }
    return Promise.resolve(apiResponse());
  });
  return calls;
}

beforeEach(() => resetDiscovery());
afterEach(() => vi.unstubAllGlobals());

describe('daemon REST client', () => {
  it('lists channels using the discovered address and bearer token', async () => {
    const calls = mockFetch(
      () => new Response(JSON.stringify({ channels: [{ platform: 'twitch', slug: 'shroud', state: 'connected' }] }), { status: 200 }),
    );
    const list = await listChannels();
    expect(list).toEqual([{ platform: 'twitch', slug: 'shroud', state: 'connected' }]);

    const api = calls.find((c) => c.url.includes('/v1/channels'))!;
    expect(api.url).toBe('http://127.0.0.1:9999/v1/channels');
    expect(new Headers(api.init?.headers).get('Authorization')).toBe('Bearer tok-xyz');
  });

  it('joins by POSTing platform and slug as JSON', async () => {
    const calls = mockFetch(() => new Response(null, { status: 204 }));
    await joinChannel('kick', 'someone');
    const post = calls.find((c) => c.init?.method === 'POST')!;
    expect(post.url).toBe('http://127.0.0.1:9999/v1/channels');
    expect(JSON.parse(post.init!.body as string)).toMatchObject({ platform: 'kick', slug: 'someone' });
    expect(new Headers(post.init?.headers).get('Content-Type')).toBe('application/json');
  });

  it('leaves via DELETE with url-encoded query params', async () => {
    const calls = mockFetch(() => new Response(null, { status: 204 }));
    await leaveChannel('twitch', 'Some Name');
    const del = calls.find((c) => c.init?.method === 'DELETE')!;
    expect(del.url).toContain('platform=twitch');
    expect(del.url).toContain('slug=Some%20Name');
  });
});
