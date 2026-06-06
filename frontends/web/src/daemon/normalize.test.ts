import { describe, expect, it } from 'vitest';
import { toFeedMessage } from './normalize';
import type { UnifiedMessage } from './wire.gen';

function message(overrides: Partial<UnifiedMessage> = {}): UnifiedMessage {
  return {
    id: '01ABC',
    platform_message_id: 'p1',
    platform: 'twitch',
    channel: { platform: 'twitch', id: '1', slug: 'shroud' },
    type: 'chat',
    author: { id: 'u1', login: 'bob', display_name: 'Bob', color: '#abcdef' },
    segments: [{ kind: 'text', text: 'hello' }],
    sent_at: '2026-06-06T12:00:00Z',
    received_at: '2026-06-06T12:00:00Z',
    ...overrides,
  };
}

describe('toFeedMessage', () => {
  it('carries id, platform, author and joins body text', () => {
    const fm = toFeedMessage(message({ segments: [{ kind: 'text', text: 'gg ' }, { kind: 'text', text: 'wp' }] }));
    expect(fm.id).toBe('01ABC');
    expect(fm.platform).toBe('twitch');
    expect(fm.author).toBe('Bob');
    expect(fm.authorColor).toBe('#abcdef');
    expect(fm.body).toBe('gg wp');
    expect(fm.ts).toMatch(/\d{1,2}:\d{2}/);
  });

  it('falls back to login and drops an empty color', () => {
    const fm = toFeedMessage(message({ author: { id: 'u', login: 'lurker', display_name: '', color: '' } }));
    expect(fm.author).toBe('lurker');
    expect(fm.authorColor).toBeUndefined();
  });

  it('builds an emote url from the provider-sized template', () => {
    const fm = toFeedMessage(
      message({
        segments: [
          {
            kind: 'emote',
            text: 'Kappa',
            emote: { provider: 'twitch', id: '25', name: 'Kappa', url_template: 'https://cdn/emote/25/{size}', animated: false },
          },
        ],
      }),
    );
    expect(fm.segments).toEqual([{ type: 'emote', code: 'Kappa', url: 'https://cdn/emote/25/2.0' }]);
  });

  it('strips @ from mentions and passes links through', () => {
    const fm = toFeedMessage(
      message({
        segments: [
          { kind: 'mention', text: '@bob' },
          { kind: 'link', text: 'https://x.com' },
        ],
      }),
    );
    expect(fm.segments).toEqual([
      { type: 'mention', user: 'bob' },
      { type: 'link', href: 'https://x.com', text: 'https://x.com' },
    ]);
  });

  it('renders cheer and masked segments as plain text', () => {
    const fm = toFeedMessage(
      message({ segments: [{ kind: 'cheer', text: 'cheer100' }, { kind: 'masked', text: '****' }] }),
    );
    expect(fm.segments).toEqual([
      { type: 'text', text: 'cheer100' },
      { type: 'text', text: '****' },
    ]);
  });
});
