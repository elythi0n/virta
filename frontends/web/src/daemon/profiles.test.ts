import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { activateProfile, listProfiles } from './profiles';
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

describe('profiles client', () => {
  it('lists profiles', async () => {
    mockFetch(() => new Response(JSON.stringify({ profiles: [{ id: '1', name: 'main', active: true, default: true }] }), { status: 200 }));
    const list = await listProfiles();
    expect(list).toEqual([{ id: '1', name: 'main', active: true, default: true }]);
  });

  it('activates a profile by id', async () => {
    const calls = mockFetch(() => new Response(null, { status: 204 }));
    await activateProfile('mod-duty');
    const post = calls.find((c) => c.init?.method === 'POST')!;
    expect(post.url).toBe('http://127.0.0.1:9999/v1/profiles/mod-duty/activate');
  });
});
