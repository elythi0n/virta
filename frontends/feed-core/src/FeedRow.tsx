import { memo, type ReactNode } from 'react';
import { clampForContrast } from './contrast';
import { EVENT_LABEL, eventCountLabel, isBigEvent, isEventType } from './events';
import PlatformGlyph from './PlatformGlyph';
import type { Segment } from './segments';
import type { FeedMessage } from './types';
import styles from './FeedRow.module.css';

export type Density = 'tiny' | 'compact' | 'cozy' | 'comfortable' | 'large';

// Known badge sets get a short label + a semantic color; others fall back to the set's initials.
const BADGE: Record<string, { label: string; color: string }> = {
  broadcaster: { label: 'HOST', color: 'var(--virta-danger)' },
  moderator: { label: 'MOD', color: 'var(--virta-ok)' },
  subscriber: { label: 'SUB', color: 'var(--virta-accent)' },
  founder: { label: 'FND', color: 'var(--virta-warn)' },
  vip: { label: 'VIP', color: 'var(--virta-warn)' },
  staff: { label: 'STAFF', color: 'var(--virta-accent)' },
  verified: { label: 'VER', color: 'var(--virta-accent)' },
};
const badgeLabel = (set: string) => BADGE[set]?.label ?? set.slice(0, 3).toUpperCase();
const badgeColor = (set: string) => BADGE[set]?.color ?? 'var(--virta-text-2)';

function renderSegment(seg: Segment, i: number) {
  switch (seg.type) {
    case 'text':
      return seg.text;
    case 'emote':
      return <img key={i} className={styles.emote} src={seg.url} alt={seg.code} title={seg.code} loading="lazy" />;
    case 'mention':
      return (
        <span key={i} className={styles.mention}>
          @{seg.user}
        </span>
      );
    case 'link':
      return (
        <a key={i} className={styles.link} href={seg.href} target="_blank" rel="noreferrer noopener">
          {seg.text}
        </a>
      );
  }
}

function SourceTag({ message }: { message: FeedMessage }) {
  if (!message.source) return null;
  return (
    <span className={styles.source} data-platform={message.platform} title={message.source.label}>
      {message.source.label}
    </span>
  );
}

type FeedRowProps = {
  message: FeedMessage;
  /** Feed background (hex) the author color is contrast-clamped against. */
  background: string;
  /** Show the source-channel attribution tag (for feeds aggregating multiple channels). */
  showSource?: boolean;
  /** Show the per-row timestamp. */
  showTimestamps?: boolean;
  /** Optional hover-revealed actions for a chat row (e.g. moderator buttons); return null to omit. */
  renderActions?: (m: FeedMessage) => ReactNode;
  density: Density;
};

// The single most-rendered element. Bespoke and memoized so streaming a new message never
// re-renders the rows above it. Chat rows carry a platform rail, optional source tag, badges, the
// author, and segments; non-chat types render as a tinted event band; deletions fade and strike.
function FeedRow({ message, background, showSource, showTimestamps = true, renderActions, density }: FeedRowProps) {
  const type = message.type ?? 'chat';

  if (isEventType(type)) {
    const big = isBigEvent(message);
    const count = eventCountLabel(message);
    return (
      <div className={`${styles.event} ${styles[`ev-${type}`]} ${styles[density]} ${big ? styles.big : ''}`}>
        <PlatformGlyph platform={message.platform} className={styles.glyph} />
        {showTimestamps && <span className={styles.ts}>{message.ts}</span>}
        {showSource && <SourceTag message={message} />}
        <span className={styles.eventLabel}>{EVENT_LABEL[type]}</span>
        {count && <span className={styles.eventCount}>{count}</span>}
        <span className={styles.eventText}>
          {message.author && <strong className={styles.eventAuthor}>{message.author} </strong>}
          {message.segments.map(renderSegment)}
        </span>
      </div>
    );
  }

  const authorStyle = message.authorColor ? { color: clampForContrast(message.authorColor, background) } : undefined;
  const badges = message.badges ?? [];
  const actions = renderActions?.(message);
  return (
    <div
      className={`${styles.row} ${styles[message.platform]} ${styles[density]} ${message.highlighted ? styles.highlight : ''} ${message.deleted ? styles.deleted : ''}`}
    >
      <PlatformGlyph platform={message.platform} className={styles.glyph} />
      {showTimestamps && <span className={styles.ts}>{message.ts}</span>}
      {showSource && <SourceTag message={message} />}
      {badges.length > 0 && (
        <span className={styles.badges}>
          {badges.slice(0, 3).map((b, i) =>
            b.url ? (
              <img key={i} className={styles.badgeImg} src={b.url} alt={b.title ?? b.set} title={b.title ?? b.set} loading="lazy" />
            ) : (
              <span key={i} className={styles.badge} style={{ color: badgeColor(b.set) }} title={b.title ?? b.set}>
                {badgeLabel(b.set)}
              </span>
            ),
          )}
          {badges.length > 3 && <span className={styles.badgeMore}>+{badges.length - 3}</span>}
        </span>
      )}
      <span className={styles.author} style={authorStyle}>
        {message.author}
      </span>
      <span className={`${styles.body} ${type === 'action' ? styles.action : ''}`}>
        {message.deleted ? <span className={styles.tombstone}>message deleted</span> : message.segments.map(renderSegment)}
      </span>
      {message.combo && message.combo > 1 && <span className={styles.combo}>×{message.combo}</span>}
      {actions && <span className={styles.rowActions}>{actions}</span>}
    </div>
  );
}

export default memo(FeedRow);
