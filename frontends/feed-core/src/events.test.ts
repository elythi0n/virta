import { describe, expect, it } from 'vitest';
import { eventCountLabel, eventHeadline, eventImpact, isBigEvent, isCelebration } from './events';
import type { FeedMessage } from './types';

const msg = (over: Partial<FeedMessage>): FeedMessage => ({
  id: 'x',
  ts: '00:00',
  platform: 'twitch',
  author: 'pylon',
  body: '',
  segments: [],
  ...over,
});

describe('eventImpact', () => {
  it('reads the right magnitude per type', () => {
    expect(eventImpact(msg({ type: 'giftsub', event: { count: 10 } }))).toBe(10);
    expect(eventImpact(msg({ type: 'raid', event: { viewers: 1240 } }))).toBe(1240);
    expect(eventImpact(msg({ type: 'resub', event: { months: 24 } }))).toBe(24);
    expect(eventImpact(msg({ type: 'chat' }))).toBe(0);
  });

  it('defaults a lone gift sub to one', () => {
    expect(eventImpact(msg({ type: 'giftsub' }))).toBe(1);
  });
});

describe('isBigEvent / isCelebration', () => {
  it('treats a gift bomb as big and celebratory', () => {
    const m = msg({ type: 'giftsub', event: { count: 10 } });
    expect(isBigEvent(m)).toBe(true);
    expect(isCelebration(m)).toBe(true);
  });

  it('treats a single gift sub as neither', () => {
    const m = msg({ type: 'giftsub', event: { count: 1 } });
    expect(isBigEvent(m)).toBe(false);
    expect(isCelebration(m)).toBe(false);
  });

  it('celebrates a sizable raid but never a lone sub', () => {
    expect(isCelebration(msg({ type: 'raid', event: { viewers: 1240 } }))).toBe(true);
    expect(isCelebration(msg({ type: 'sub' }))).toBe(false);
    expect(isCelebration(msg({ type: 'resub', event: { months: 99 } }))).toBe(false);
  });
});

describe('eventHeadline / eventCountLabel', () => {
  it('pluralizes the gift headline', () => {
    expect(eventHeadline(msg({ type: 'giftsub', event: { count: 10 } }))).toBe('pylon gifted 10 subs');
    expect(eventHeadline(msg({ type: 'giftsub', event: { count: 1 } }))).toBe('pylon gifted 1 sub');
  });

  it('compacts large viewer counts on the chip', () => {
    expect(eventCountLabel(msg({ type: 'giftsub', event: { count: 10 } }))).toBe('×10');
    expect(eventCountLabel(msg({ type: 'raid', event: { viewers: 1240 } }))).toBe('1.2k');
    expect(eventCountLabel(msg({ type: 'raid', event: { viewers: 42 } }))).toBe('42');
    expect(eventCountLabel(msg({ type: 'resub', event: { months: 24 } }))).toBe('24mo');
    expect(eventCountLabel(msg({ type: 'chat' }))).toBe('');
  });
});
