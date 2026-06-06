import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { previewSend, sendMessage } from './send';
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

describe('send client', () => {
  it('previews reachability for the given channels', async () => {
    mockFetch(
      () => new Response(JSON.stringify({ targets: [{ channel: 'twitch:a', can_send: true }, { channel: 'kick:b', can_send: false, reason: 'not_authenticated' }] }), { status: 200 }),
    );
    const targets = await previewSend(['twitch:a', 'kick:b']);
    expect(targets).toHaveLength(2);
    expect(targets[1]).toMatchObject({ channel: 'kick:b', can_send: false, reason: 'not_authenticated' });
  });

  it('sends text to the reachable channels and returns dispositions', async () => {
    const calls = mockFetch(() => new Response(JSON.stringify({ results: [{ channel: 'twitch:a', status: 'sent' }] }), { status: 200 }));
    const results = await sendMessage(['twitch:a'], 'hello');
    expect(results).toEqual([{ channel: 'twitch:a', status: 'sent' }]);
    const post = calls.find((c) => c.url.endsWith('/v1/send'))!;
    expect(JSON.parse(post.init!.body as string)).toEqual({ channels: ['twitch:a'], text: 'hello' });
  });
});
