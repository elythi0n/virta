// Keyboard shortcut specs are '+'-joined and lowercase, e.g. 'mod+shift+p'. `mod` is Ctrl on
// Windows/Linux and Cmd on macOS, so one spec works everywhere.

const isMac = typeof navigator !== 'undefined' && /mac/i.test(navigator.platform);

/** Does a keydown event match the spec? The non-`mod` platform modifier must NOT be held. */
export function matchesShortcut(e: KeyboardEvent, spec: string): boolean {
  const parts = spec.toLowerCase().split('+');
  const key = parts[parts.length - 1];
  const wantMod = parts.includes('mod');
  const wantShift = parts.includes('shift');
  const wantAlt = parts.includes('alt');

  const modDown = isMac ? e.metaKey : e.ctrlKey;
  const otherModDown = isMac ? e.ctrlKey : e.metaKey;

  if (wantMod !== modDown || otherModDown) return false;
  if (wantShift !== e.shiftKey) return false;
  if (wantAlt !== e.altKey) return false;
  return e.key.toLowerCase() === key;
}

/** Render a spec for display, e.g. 'mod+shift+p' → '⌘⇧P' (mac) or 'Ctrl+Shift+P'. */
export function formatShortcut(spec: string): string {
  const parts = spec.toLowerCase().split('+');
  const sym: Record<string, string> = {
    mod: isMac ? '⌘' : 'Ctrl',
    shift: isMac ? '⇧' : 'Shift',
    alt: isMac ? '⌥' : 'Alt',
  };
  const sep = isMac ? '' : '+';
  return parts.map((p, i) => (i === parts.length - 1 ? p.toUpperCase() : (sym[p] ?? p))).join(sep);
}
