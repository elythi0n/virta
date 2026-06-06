import { describe, expect, it } from 'vitest';
import { formatShortcut, matchesShortcut } from './shortcut';

// In the node test env there's no `navigator`, so `mod` resolves to Ctrl (non-mac path).
const ev = (e: Partial<KeyboardEvent>) =>
  ({ ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, ...e }) as KeyboardEvent;

describe('matchesShortcut', () => {
  it('matches a mod+shift+key combo', () => {
    expect(matchesShortcut(ev({ ctrlKey: true, shiftKey: true, key: 'P' }), 'mod+shift+p')).toBe(true);
  });
  it('matches mod+key', () => {
    expect(matchesShortcut(ev({ ctrlKey: true, key: 'b' }), 'mod+b')).toBe(true);
  });
  it('rejects when an extra modifier is held', () => {
    expect(matchesShortcut(ev({ ctrlKey: true, shiftKey: true, key: 'b' }), 'mod+b')).toBe(false);
  });
  it('rejects when mod is missing', () => {
    expect(matchesShortcut(ev({ key: 'b' }), 'mod+b')).toBe(false);
  });
  it('handles punctuation keys', () => {
    expect(matchesShortcut(ev({ ctrlKey: true, key: ',' }), 'mod+,')).toBe(true);
  });
});

describe('formatShortcut', () => {
  it('renders the non-mac form', () => {
    expect(formatShortcut('mod+shift+p')).toBe('Ctrl+Shift+P');
    expect(formatShortcut('mod+b')).toBe('Ctrl+B');
  });
});
