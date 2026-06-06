import { describe, expect, it } from 'vitest';
import { appendCapped, clearIn, markDeletedIn } from './useFeedBuffer';
import type { FeedMessage } from './types';

const msg = (id: string, over: Partial<FeedMessage> = {}): FeedMessage => ({
  id,
  ts: '00:00',
  platform: 'twitch',
  author: 'a',
  body: id,
  segments: [{ type: 'text', text: id }],
  ...over,
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

describe('markDeletedIn', () => {
  it('strikes the message matching the engine id', () => {
    const out = markDeletedIn([msg('1'), msg('2')], { id: '2' });
    expect(out.map((m) => m.deleted)).toEqual([undefined, true]);
  });

  it('falls back to the platform message id when the engine id is empty', () => {
    const msgs = [msg('1', { platformMessageId: 'p1' }), msg('2', { platformMessageId: 'p2' })];
    const out = markDeletedIn(msgs, { platformMessageId: 'p2' });
    expect(out[1].deleted).toBe(true);
    expect(out[0].deleted).toBeUndefined();
  });

  it('returns the same array reference when nothing matched', () => {
    const prev = [msg('1'), msg('2')];
    expect(markDeletedIn(prev, { id: 'missing' })).toBe(prev);
  });
});

describe('clearIn', () => {
  const ch = 'twitch:forsen';
  const msgs = () => [
    msg('1', { channel: ch, authorId: 'u1' }),
    msg('2', { channel: ch, authorId: 'u2' }),
    msg('3', { channel: 'twitch:shroud', authorId: 'u1' }),
  ];

  it('a user timeout strikes only that user in that channel', () => {
    const out = clearIn(msgs(), ch, 'u1');
    expect(out.map((m) => m.deleted)).toEqual([true, undefined, undefined]);
  });

  it('a full clear (no user) strikes every message in that channel only', () => {
    const out = clearIn(msgs(), ch);
    expect(out.map((m) => m.deleted)).toEqual([true, true, undefined]);
  });

  it('returns the same array reference when the channel has no messages', () => {
    const prev = msgs();
    expect(clearIn(prev, 'kick:xqc')).toBe(prev);
  });
});
