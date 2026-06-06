import { describe, expect, it } from 'vitest';
import type { FeedMessage } from '@virta/feed-core';
import { filterFeed } from './quickFilter';

const msg = (over: Partial<FeedMessage>): FeedMessage => ({
  id: Math.random().toString(36),
  ts: '00:00',
  platform: 'twitch',
  author: 'viewer',
  body: '',
  segments: [],
  ...over,
});

const feed: FeedMessage[] = [
  msg({ author: 'alice', body: 'hello world' }),
  msg({ type: 'sub', author: 'bob', body: 'subscribed' }),
  msg({ type: 'giftsub', author: 'carol', body: 'gifted 10' }),
  msg({ type: 'raid', author: 'dave', body: 'raiding' }),
  msg({ author: 'mod_mary', body: 'cleanup', badges: [{ set: 'moderator' }] }),
];

describe('filterFeed', () => {
  it('returns the same array reference for the default view', () => {
    expect(filterFeed(feed, 'all', '')).toBe(feed);
  });

  it('subs includes sub, resub, and gift', () => {
    expect(filterFeed(feed, 'subs', '').map((m) => m.author)).toEqual(['bob', 'carol']);
  });

  it('gifts is only gift subs', () => {
    expect(filterFeed(feed, 'gifts', '').map((m) => m.author)).toEqual(['carol']);
  });

  it('raids includes raids and hosts', () => {
    expect(filterFeed(feed, 'raids', '').map((m) => m.author)).toEqual(['dave']);
  });

  it('mods matches a moderator or broadcaster badge', () => {
    expect(filterFeed(feed, 'mods', '').map((m) => m.author)).toEqual(['mod_mary']);
  });

  it('search matches author or body, case-insensitively', () => {
    expect(filterFeed(feed, 'all', 'HELLO').map((m) => m.author)).toEqual(['alice']);
    expect(filterFeed(feed, 'all', 'carol').map((m) => m.author)).toEqual(['carol']);
  });

  it('combines a quick filter with search (AND)', () => {
    expect(filterFeed(feed, 'subs', 'gift').map((m) => m.author)).toEqual(['carol']);
    expect(filterFeed(feed, 'subs', 'alice')).toHaveLength(0);
  });
});
