// Username contrast clamp. Platform-provided author colors can be unreadable on our backgrounds
// (a near-black name on the dark theme). We keep the hue but push lightness toward the readable
// end until the color clears a WCAG contrast ratio against the feed background.

type Rgb = { r: number; g: number; b: number };

function hexToRgb(hex: string): Rgb {
  let h = hex.trim().replace(/^#/, '');
  if (h.length === 3) h = h.split('').map((c) => c + c).join('');
  const n = parseInt(h, 16);
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
}

function toHex({ r, g, b }: Rgb): string {
  return '#' + [r, g, b].map((c) => Math.round(c).toString(16).padStart(2, '0')).join('').toUpperCase();
}

function channel(c: number): number {
  const s = c / 255;
  return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
}

function luminance({ r, g, b }: Rgb): number {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

function ratio(a: Rgb, b: Rgb): number {
  const la = luminance(a);
  const lb = luminance(b);
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}

function mix(a: Rgb, b: Rgb, t: number): Rgb {
  return { r: a.r + (b.r - a.r) * t, g: a.g + (b.g - a.g) * t, b: a.b + (b.b - a.b) * t };
}

/** WCAG contrast ratio (1..21) between two hex colors. */
export function contrastRatio(fg: string, bg: string): number {
  return ratio(hexToRgb(fg), hexToRgb(bg));
}

/**
 * Return `color` if it already meets `minRatio` against `background`, else blend it toward white
 * (on a dark background) or black (on a light one) just until it does.
 */
export function clampForContrast(color: string, background: string, minRatio = 4.5): string {
  const bg = hexToRgb(background);
  const start = hexToRgb(color);
  if (ratio(start, bg) >= minRatio) return toHex(start);

  const target: Rgb = luminance(bg) < 0.5 ? { r: 255, g: 255, b: 255 } : { r: 0, g: 0, b: 0 };
  for (let t = 0.05; t <= 1.0001; t += 0.05) {
    const candidate = mix(start, target, t);
    if (ratio(candidate, bg) >= minRatio) return toHex(candidate);
  }
  return toHex(target);
}
