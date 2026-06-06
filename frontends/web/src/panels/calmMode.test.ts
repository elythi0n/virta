import { describe, expect, it } from 'vitest';
import type { FeedMessage } from '@virta/feed-core';
import { collapseCombos } from './calmMode';

const msg = (id: string, body: string, over: Partial<FeedMessage> = {}): FeedMessage => ({
  id,
  ts: '00:00',
  platform: 'twitch',
  author: id,
  body,
  segments: [{ type: 'text', text: body }],
  ...over,
});

describe('collapseCombos', () => {
  it('folds a run of identical messages into one row with a count', () => {
    const out = collapseCombos([msg('1', 'LUL'), msg('2', 'LUL'), msg('3', 'LUL')]);
    expect(out).toHaveLength(1);
    expect(out[0].id).toBe('1'); // first stays as the representative (stable key)
    expect(out[0].combo).toBe(3);
  });

  it('keeps distinct messages separate', () => {
    const out = collapseCombos([msg('1', 'hi'), msg('2', 'LUL'), msg('3', 'LUL'), msg('4', 'bye')]);
    expect(out.map((m) => m.body)).toEqual(['hi', 'LUL', 'bye']);
    expect(out.map((m) => m.combo)).toEqual([undefined, 2, undefined]);
  });

  it('never collapses across an event or a deleted message', () => {
    const out = collapseCombos([msg('1', 'LUL'), msg('2', 'gifted', { type: 'giftsub' }), msg('3', 'LUL')]);
    expect(out).toHaveLength(3);
  });

  it('preserves object identity for uncollapsed rows', () => {
    const a = msg('1', 'hi');
    const b = msg('2', 'bye');
    const out = collapseCombos([a, b]);
    expect(out[0]).toBe(a);
    expect(out[1]).toBe(b);
  });

  it('ignores blank bodies', () => {
    const out = collapseCombos([msg('1', '  '), msg('2', '  ')]);
    expect(out).toHaveLength(2);
  });
});
