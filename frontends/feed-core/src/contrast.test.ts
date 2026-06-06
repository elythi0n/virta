import { describe, expect, it } from 'vitest';
import { clampForContrast, contrastRatio } from './contrast';

const DARK_BG = '#0E0F12';
const LIGHT_BG = '#FAFBFC';

describe('contrastRatio', () => {
  it('is ~21 for black on white', () => {
    expect(contrastRatio('#FFFFFF', '#000000')).toBeCloseTo(21, 0);
  });
  it('is 1 for a color against itself', () => {
    expect(contrastRatio('#777777', '#777777')).toBeCloseTo(1, 5);
  });
  it('accepts shorthand hex', () => {
    expect(contrastRatio('#FFF', '#000')).toBeCloseTo(21, 0);
  });
});

describe('clampForContrast', () => {
  it('leaves a color that already passes untouched', () => {
    expect(clampForContrast('#FFFFFF', DARK_BG)).toBe('#FFFFFF');
  });

  it('lightens a too-dark name on a dark background until it passes', () => {
    const out = clampForContrast('#111111', DARK_BG);
    expect(contrastRatio(out, DARK_BG)).toBeGreaterThanOrEqual(4.5);
  });

  it('darkens a too-light name on a light background until it passes', () => {
    const out = clampForContrast('#EEEEEE', LIGHT_BG);
    expect(contrastRatio(out, LIGHT_BG)).toBeGreaterThanOrEqual(4.5);
  });

  it('respects a custom minimum ratio', () => {
    const out = clampForContrast('#5B8CFF', DARK_BG, 7);
    expect(contrastRatio(out, DARK_BG)).toBeGreaterThanOrEqual(7);
  });
});
