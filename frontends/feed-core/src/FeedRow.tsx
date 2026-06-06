import { memo } from 'react';
import { clampForContrast } from './contrast';
import type { Segment } from './segments';
import type { FeedMessage } from './types';
import styles from './FeedRow.module.css';

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
};

// The single most-rendered element. Bespoke (not a library primitive) and memoized so streaming a
// new message never re-renders the rows above it. A left rail carries platform identity; an
// optional source tag names the channel when several are merged into one feed.
function FeedRow({ message, background, showSource }: FeedRowProps) {
  const authorStyle = message.authorColor
    ? { color: clampForContrast(message.authorColor, background) }
    : undefined;
  return (
    <div className={`${styles.row} ${styles[message.platform]}`}>
      <span className={styles.ts}>{message.ts}</span>
      {showSource && message.source && (
        <span className={styles.source} data-platform={message.platform} title={message.source.label}>
          {message.source.label}
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
