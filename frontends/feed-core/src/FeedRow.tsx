import { memo } from 'react';
import { clampForContrast } from './contrast';
import type { Segment } from './segments';
import type { FeedMessage } from './types';
import styles from './FeedRow.module.css';

export type Density = 'compact' | 'cozy' | 'comfortable';

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

type FeedRowProps = {
  message: FeedMessage;
  /** Feed background (hex) the author color is contrast-clamped against. */
  background: string;
  /** Show the source-channel attribution tag (for feeds aggregating multiple channels). */
  showSource?: boolean;
  density: Density;
};

// The single most-rendered element. Bespoke (not a library primitive) and memoized so streaming a
// new message never re-renders the rows above it. A left rail carries platform identity; an
// optional source tag names the channel when several are merged; badges mark the author's role.
function FeedRow({ message, background, showSource, density }: FeedRowProps) {
  const authorStyle = message.authorColor
    ? { color: clampForContrast(message.authorColor, background) }
    : undefined;
  const badges = message.badges ?? [];
  return (
    <div className={`${styles.row} ${styles[message.platform]} ${styles[density]}`}>
      <span className={styles.ts}>{message.ts}</span>
      {showSource && message.source && (
        <span className={styles.source} data-platform={message.platform} title={message.source.label}>
          {message.source.label}
        </span>
      )}
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
      <span className={styles.body}>{message.segments.map(renderSegment)}</span>
    </div>
  );
}

export default memo(FeedRow);
