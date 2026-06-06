import { describe, expect, it } from 'vitest';
import type { FeedMessage } from '@virta/feed-core';
import { applyCalm, collapseCombos } from './calmMode';

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

  it('folds an emote wall of differing emotes into one combo', () => {
    const emote = (id: string, code: string): FeedMessage => ({
      ...msg(id, code),
      segments: [{ type: 'emote', code, url: `/${code}.png` }],
    });
    const out = collapseCombos([emote('1', 'KEKW'), emote('2', 'LUL'), emote('3', 'Pog')]);
    expect(out).toHaveLength(1);
    expect(out[0].id).toBe('1'); // first emote-only message represents the wall
    expect(out[0].combo).toBe(3);
  });

  it('does not fold an emote message into a text message', () => {
    const emote: FeedMessage = { ...msg('1', 'KEKW'), segments: [{ type: 'emote', code: 'KEKW', url: '/k.png' }] };
    const text = msg('2', 'hello');
    const out = collapseCombos([emote, text]);
    expect(out).toHaveLength(2);
  });

  it('folds emote-with-whitespace as emote-only but not emote-with-words', () => {
    const wall1: FeedMessage = {
      ...msg('1', 'KEKW '),
      segments: [
        { type: 'emote', code: 'KEKW', url: '/k.png' },
        { type: 'text', text: ' ' },
      ],
    };
    const wall2: FeedMessage = { ...msg('2', 'LUL'), segments: [{ type: 'emote', code: 'LUL', url: '/l.png' }] };
    const worded: FeedMessage = {
      ...msg('3', 'lol KEKW'),
      segments: [
        { type: 'text', text: 'lol ' },
        { type: 'emote', code: 'KEKW', url: '/k.png' },
      ],
    };
    const out = collapseCombos([wall1, wall2, worded]);
    expect(out.map((m) => m.id)).toEqual(['1', '3']); // wall1+wall2 fold; worded stays separate
    expect(out[0].combo).toBe(2);
  });
});

describe('applyCalm', () => {
  it('drops sampled messages and reports how many were thinned', () => {
    const { visible, thinned } = applyCalm([
      msg('1', 'hi'),
      msg('2', 'spam', { sampled: true }),
      msg('3', 'spam', { sampled: true }),
      msg('4', 'bye'),
    ]);
    expect(visible.map((m) => m.body)).toEqual(['hi', 'bye']);
    expect(thinned).toBe(2);
  });

  it('keeps unsampled messages and still collapses their repeats', () => {
    const { visible, thinned } = applyCalm([msg('1', 'LUL'), msg('2', 'LUL'), msg('3', 'LUL', { sampled: true })]);
    expect(visible).toHaveLength(1);
    expect(visible[0].combo).toBe(2); // the sampled third is dropped before collapsing
    expect(thinned).toBe(1);
  });

  it('thins nothing when no message is sampled', () => {
    const { visible, thinned } = applyCalm([msg('1', 'hi'), msg('2', 'bye')]);
    expect(visible).toHaveLength(2);
    expect(thinned).toBe(0);
  });
});
