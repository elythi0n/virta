import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { approveHeld, denyHeld, listHeld } from './held';
import { resetDiscovery } from './discovery';

const DISCOVERY = { addr: '127.0.0.1:9999', token: 'tok' };
type Call = { url: string; init?: RequestInit };

function mockFetch(apiResponse: () => Response): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, init });
    if (url.endsWith('/__discovery')) return Promise.resolve(new Response(JSON.stringify(DISCOVERY), { status: 200 }));
    return Promise.resolve(apiResponse());
  });
  return calls;
}

beforeEach(() => resetDiscovery());
afterEach(() => vi.unstubAllGlobals());

describe('held client', () => {
  it('lists the hold queue', async () => {
    mockFetch(() => new Response(JSON.stringify({ held: [{ id: 'h1', channel: 'twitch:a', author: 'bob', text: 'sus', reason: 'harassment', held_at_ms: 0 }] }), { status: 200 }));
    const held = await listHeld();
    expect(held).toHaveLength(1);
    expect(held[0]).toMatchObject({ id: 'h1', channel: 'twitch:a', reason: 'harassment' });
  });

  it('approves a held message by id with a POST', async () => {
    const calls = mockFetch(() => new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await approveHeld('h 1'); // id needs encoding
    const post = calls.find((c) => c.url.includes('/v1/held/'))!;
    expect(post.url).toContain('/v1/held/h%201/approve');
    expect(post.init!.method).toBe('POST');
  });

  it('denies a held message by id with a POST', async () => {
    const calls = mockFetch(() => new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await denyHeld('h2');
    const post = calls.find((c) => c.url.includes('/v1/held/'))!;
    expect(post.url).toContain('/v1/held/h2/deny');
    expect(post.init!.method).toBe('POST');
  });
});
