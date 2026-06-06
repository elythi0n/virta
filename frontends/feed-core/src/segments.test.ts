import { describe, expect, it } from 'vitest';
import { parseSegments } from './segments';

describe('parseSegments', () => {
  it('keeps plain text as a single coalesced segment', () => {
    expect(parseSegments('hello there friend')).toEqual([{ type: 'text', text: 'hello there friend' }]);
  });

  it('replaces known emote codes, preserving surrounding spaces', () => {
    expect(parseSegments('lol Kappa gg', { Kappa: { url: 'u' } })).toEqual([
      { type: 'text', text: 'lol ' },
      { type: 'emote', code: 'Kappa', url: 'u' },
      { type: 'text', text: ' gg' },
    ]);
  });

  it('leaves an unknown emote-looking word as text', () => {
    expect(parseSegments('Kappa', {})).toEqual([{ type: 'text', text: 'Kappa' }]);
  });

  it('extracts mentions without the leading @', () => {
    expect(parseSegments('hey @bob nice')).toEqual([
      { type: 'text', text: 'hey ' },
      { type: 'mention', user: 'bob' },
      { type: 'text', text: ' nice' },
    ]);
  });

  it('extracts links', () => {
    expect(parseSegments('see https://x.com now')).toEqual([
      { type: 'text', text: 'see ' },
      { type: 'link', href: 'https://x.com', text: 'https://x.com' },
      { type: 'text', text: ' now' },
    ]);
  });

  it('handles a mix and a leading token', () => {
    expect(parseSegments('@a hi Kappa', { Kappa: { url: 'u' } })).toEqual([
      { type: 'mention', user: 'a' },
      { type: 'text', text: ' hi ' },
      { type: 'emote', code: 'Kappa', url: 'u' },
    ]);
  });
});
