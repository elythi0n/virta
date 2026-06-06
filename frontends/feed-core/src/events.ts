import type { FeedMessage, MessageType } from './types';

// Above these magnitudes an event is "high impact": it earns a stronger band in the scrollback
// and a one-shot celebration banner when it arrives live.
export const HIGH_IMPACT_GIFTS = 5;
export const HIGH_IMPACT_VIEWERS = 50;

// Rough size of an event, used to pick the bigger of several arriving at once and to scale the
// band. 0 for ordinary chat / unsized events; larger means a bigger splash.
export function eventImpact(m: FeedMessage): number {
  switch (m.type) {
    case 'giftsub':
      return m.event?.count ?? 1;
    case 'raid':
    case 'host':
      return m.event?.viewers ?? 0;
    case 'resub':
      return m.event?.months ?? 0;
    default:
      return 0;
  }
}

// A "big" event band: visually heavier than an ordinary sub/follow notice.
export function isBigEvent(m: FeedMessage): boolean {
  switch (m.type) {
    case 'giftsub':
      return (m.event?.count ?? 1) >= HIGH_IMPACT_GIFTS;
    case 'raid':
    case 'host':
      return (m.event?.viewers ?? 0) >= HIGH_IMPACT_VIEWERS;
    default:
      return false;
  }
}

// Whether an arriving event deserves the live celebration banner. Same bar as a big band, but
// only the few genuinely celebratory types (a lone sub or follow never interrupts).
export function isCelebration(m: FeedMessage): boolean {
  return (m.type === 'giftsub' || m.type === 'raid' || m.type === 'host') && isBigEvent(m);
}

// One-line headline for the celebration banner.
export function eventHeadline(m: FeedMessage): string {
  const who = m.author || 'Someone';
  switch (m.type) {
    case 'giftsub': {
      const n = m.event?.count ?? 1;
      return `${who} gifted ${n} sub${n === 1 ? '' : 's'}`;
    }
    case 'raid':
      return `${who} raided with ${m.event?.viewers ?? 0} viewers`;
    case 'host':
      return `${who} hosted with ${m.event?.viewers ?? 0} viewers`;
    default:
      return who;
  }
}

// Short magnitude chip shown inline on the event band ("×10", "4.2k"), or "" when unsized.
export function eventCountLabel(m: FeedMessage): string {
  switch (m.type) {
    case 'giftsub':
      return m.event?.count ? `×${m.event.count}` : '';
    case 'raid':
    case 'host':
      return m.event?.viewers ? compact(m.event.viewers) : '';
    case 'resub':
      return m.event?.months ? `${m.event.months}mo` : '';
    default:
      return '';
  }
}

function compact(n: number): string {
  if (n >= 1000) {
    const k = n / 1000;
    return `${k >= 10 ? Math.round(k) : k.toFixed(1).replace(/\.0$/, '')}k`;
  }
  return String(n);
}

export type { MessageType };
