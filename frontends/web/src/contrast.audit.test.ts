import { describe, expect, it } from 'vitest';
import { contrastRatio } from '@virta/feed-core';
import tokens from '../../ui-kit/tokens.json';

// Automated contrast audit: every theme's core foreground/background token pairs must clear WCAG
// thresholds, so neither the dark nor the light palette can drift into unreadable territory. Reads
// the token source of truth (ui-kit/tokens.json) so a bad edit fails the test, not just the eye.
const THEMES = tokens.themes as Record<string, { color: Record<string, string> }>;

// AA: 4.5 for body text, 3.0 for large/secondary text and UI accents.
const PAIRS: { fg: string; bg: string; min: number; what: string }[] = [
  { fg: 'text-0', bg: 'bg-0', min: 4.5, what: 'primary text on base' },
  { fg: 'text-1', bg: 'bg-0', min: 4.5, what: 'secondary text on base' },
  { fg: 'text-2', bg: 'bg-0', min: 3.0, what: 'muted text on base' },
  { fg: 'accent', bg: 'bg-0', min: 3.0, what: 'accent on base' },
];

describe('theme contrast audit', () => {
  for (const [name, theme] of Object.entries(THEMES)) {
    it(`${name} theme has readable token pairs`, () => {
      for (const p of PAIRS) {
        const fg = theme.color[p.fg];
        const bg = theme.color[p.bg];
        expect(fg, `${p.fg} defined`).toBeTruthy();
        expect(bg, `${p.bg} defined`).toBeTruthy();
        const r = contrastRatio(fg, bg);
        expect(r, `${name}: ${p.what} (${fg} on ${bg}) = ${r.toFixed(2)}:1, want ≥ ${p.min}`).toBeGreaterThanOrEqual(p.min);
      }
    });
  }
});
