import { describe, expect, it } from 'vitest';
import { applySuggestion, suggest, tokenAt, type EmoteEntry } from './autocomplete';

const emote = (code: string): EmoteEntry => ({ code, url: `u/${code}`, lc: code.toLowerCase() });
const EMOTES = ['Kappa', 'KappaPride', 'PogChamp', 'LUL'].map(emote);

describe('tokenAt', () => {
  it('returns the word ending at the caret and its start', () => {
    expect(tokenAt('hello Kap', 9)).toEqual({ token: 'Kap', start: 6 });
    expect(tokenAt('hello ', 6)).toEqual({ token: '', start: 6 });
  });
});

describe('suggest', () => {
  it('matches emotes by prefix first, then substring', () => {
    const out = suggest('kap', EMOTES, []);
    expect(out.map((s) => s.value)).toEqual(['Kappa', 'KappaPride']);
  });

  it('needs at least two characters for emotes', () => {
    expect(suggest('K', EMOTES, [])).toHaveLength(0);
  });

  it('skips an exact match', () => {
    expect(suggest('LUL', EMOTES, []).map((s) => s.value)).not.toContain('LUL');
  });

  it('suggests chatters for an @ token', () => {
    const out = suggest('@ali', [], ['alice', 'bob', 'Alistair']);
    expect(out.map((s) => s.value)).toEqual(['@alice', '@Alistair']);
  });
});

describe('applySuggestion', () => {
  it('replaces the token and adds a trailing space', () => {
    const r = applySuggestion('gg Kap', 6, 'Kappa');
    expect(r.text).toBe('gg Kappa ');
    expect(r.caret).toBe(9);
  });

  it('keeps text after the caret', () => {
    const r = applySuggestion('Kap end', 3, 'Kappa');
    expect(r.text).toBe('Kappa  end');
  });
});
