import { describe, expect, it } from 'vitest';
import { appendCapped } from './useFeedBuffer';
import type { FeedMessage } from './types';

const msg = (id: string): FeedMessage => ({
  id,
  ts: '00:00',
  platform: 'twitch',
  author: 'a',
  body: id,
  segments: [{ type: 'text', text: id }],
});

describe('appendCapped', () => {
  it('returns prev unchanged when nothing is incoming', () => {
    const prev = [msg('1')];
    expect(appendCapped(prev, [], 10)).toBe(prev);
  });

  it('appends in arrival order', () => {
    const out = appendCapped([msg('1')], [msg('2'), msg('3')], 10);
    expect(out.map((m) => m.id)).toEqual(['1', '2', '3']);
  });

  it('keeps only the newest `max`, dropping the oldest', () => {
    const out = appendCapped([msg('1'), msg('2')], [msg('3'), msg('4')], 3);
    expect(out.map((m) => m.id)).toEqual(['2', '3', '4']);
  });

  it('caps a first batch larger than max to the newest', () => {
    const out = appendCapped([], [msg('1'), msg('2'), msg('3')], 2);
    expect(out.map((m) => m.id)).toEqual(['2', '3']);
  });
});
