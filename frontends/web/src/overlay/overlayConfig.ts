// Overlay configuration — parsed from URL query params.
// Every overlay type shares the same parameter schema so one overlay.html handles all kinds.

export type OverlayKind = 'chat' | 'mentions' | 'celebrations' | 'events';

export type OverlayTheme =
  | 'transparent'   // fully transparent bg — classic OBS source look
  | 'frosted-dark'  // semi-transparent dark glass
  | 'frosted-light' // semi-transparent light glass
  | 'solid-dark'    // opaque dark (for windowed capture)
  | 'solid-light';  // opaque light

export type OverlayAlign = 'bottom' | 'top' | 'left' | 'right';
export type OverlayDensity = 'compact' | 'cozy' | 'comfortable' | 'large';

export interface OverlayConfig {
  kind: OverlayKind;
  /** When set, render the named panel kind directly instead of the legacy feed view. */
  panelId?: string;
  channels: string[];         // "platform:slug" list; empty = all
  token: string;
  theme: OverlayTheme;
  density: OverlayDensity;
  maxMessages: number;        // how many messages to keep in the buffer
  showSource: boolean;        // show channel attribution tag
  showTimestamps: boolean;
  showBadges: boolean;
  fadeMs: number;             // 0 = no fade; >0 = fade out after N ms (good for in-game overlay)
  align: OverlayAlign;        // which edge messages attach to
  width: number;              // 0 = 100vw
  height: number;             // 0 = 100vh
  fontSize: number;           // override px; 0 = theme default
  textShadow: boolean;        // crisp text on busy scenes
  transparent: boolean;       // transparent background (for OBS browser source)
}

export const OVERLAY_DEFAULTS: Omit<OverlayConfig, 'token' | 'channels'> = {
  kind: 'chat',
  theme: 'transparent',
  density: 'comfortable',
  maxMessages: 80,
  showSource: false,
  showTimestamps: false,
  showBadges: true,
  fadeMs: 0,
  align: 'bottom',
  width: 0,
  height: 0,
  fontSize: 0,
  textShadow: true,
  transparent: false,
};

export function parseOverlayConfig(): OverlayConfig {
  const p = new URLSearchParams(location.search);
  const g = <T>(key: string, def: T, parse: (v: string) => T): T => {
    const v = p.get(key);
    return v !== null ? parse(v) : def;
  };
  return {
    kind: g('kind', 'chat', v => v as OverlayKind),
    panelId: p.get('panel') ?? undefined,
    channels: p.get('channels')?.split(',').filter(Boolean) ?? [],
    token: p.get('token') ?? '',
    theme: g('theme', 'transparent', v => v as OverlayTheme),
    density: g('density', 'comfortable', v => v as OverlayDensity),
    maxMessages: g('max', 80, v => Math.min(500, Math.max(10, parseInt(v, 10)))),
    showSource: g('source', false, v => v === '1'),
    showTimestamps: g('timestamps', false, v => v === '1'),
    showBadges: g('badges', true, v => v !== '0'),
    fadeMs: g('fade', 0, v => Math.max(0, parseInt(v, 10))),
    align: g('align', 'bottom', v => v as OverlayAlign),
    width: g('width', 0, v => parseInt(v, 10)),
    height: g('height', 0, v => parseInt(v, 10)),
    fontSize: g('fontsize', 0, v => parseInt(v, 10)),
    textShadow: g('shadow', true, v => v !== '0'),
    transparent: g('transparent', false, v => v === '1'),
  };
}

export function buildOverlayUrl(base: string, cfg: Partial<OverlayConfig> & { token: string; channels?: string[] }): string {
  const p = new URLSearchParams();
  const d = OVERLAY_DEFAULTS;
  p.set('token', cfg.token);
  if (cfg.channels?.length) p.set('channels', cfg.channels.join(','));
  if (cfg.panelId) p.set('panel', cfg.panelId);
  if (cfg.kind && cfg.kind !== d.kind) p.set('kind', cfg.kind);
  if (cfg.theme && cfg.theme !== d.theme) p.set('theme', cfg.theme);
  if (cfg.density && cfg.density !== d.density) p.set('density', cfg.density);
  if (cfg.maxMessages && cfg.maxMessages !== d.maxMessages) p.set('max', String(cfg.maxMessages));
  if (cfg.showSource) p.set('source', '1');
  if (cfg.showTimestamps) p.set('timestamps', '1');
  if (cfg.showBadges === false) p.set('badges', '0');
  if (cfg.fadeMs) p.set('fade', String(cfg.fadeMs));
  if (cfg.align && cfg.align !== d.align) p.set('align', cfg.align);
  if (cfg.width) p.set('width', String(cfg.width));
  if (cfg.height) p.set('height', String(cfg.height));
  if (cfg.fontSize) p.set('fontsize', String(cfg.fontSize));
  if (cfg.textShadow === false) p.set('shadow', '0');
  if (cfg.transparent) p.set('transparent', '1');
  return `${base}/overlay?${p.toString()}`;
}
