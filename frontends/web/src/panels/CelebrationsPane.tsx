import { useCallback, useState } from 'react';
import {
  EVENT_LABEL,
  eventCountLabel,
  isBigEvent,
  isEventType,
  PlatformGlyph,
  type FeedMessage,
  type Platform,
} from '@virta/feed-core';
import { Text } from '@virta/ui-kit';
import { useDaemonStream } from '../daemon';
import styles from './CelebrationsPane.module.css';

const MAX = 40; // events are rare; a small capped list needs no virtualization

// A calm shoutout wall: subs, gifts, raids, and announcements across every channel, each arriving
// as a card with a subtle one-shot entrance. Derived from the same stream as chat, filtered to the
// celebratory message types. Newest on top so the latest shoutout is always in view.
export default function CelebrationsPane() {
  const [events, setEvents] = useState<FeedMessage[]>([]);

  const onMessage = useCallback((m: FeedMessage) => {
    if (isEventType(m.type)) setEvents((prev) => [m, ...prev].slice(0, MAX));
  }, []);
  useDaemonStream({ onMessage });

  if (events.length === 0) {
    return (
      <div className={styles.empty}>
        <Text variant="ui" tone="subtle">
          Subs, gifts, raids, and announcements will celebrate here.
        </Text>
      </div>
    );
  }

  return (
    <div className={styles.pane}>
      {events.map((m) => {
        const type = m.type ?? 'system';
        const count = eventCountLabel(m);
        return (
          <article key={m.id} className={styles.card} data-type={type} data-big={isBigEvent(m)}>
            <span className={styles.glow} aria-hidden />
            <div className={styles.head}>
              <PlatformGlyph platform={m.platform as Platform} className={styles.glyph} />
              <span className={styles.label}>{EVENT_LABEL[type] ?? 'EVENT'}</span>
              {count && <span className={styles.count}>{count}</span>}
              <span className={styles.meta}>
                {m.source?.label ?? ''}
                {m.source ? ' · ' : ''}
                {m.ts}
              </span>
            </div>
            <div className={styles.body}>
              {m.author && <span className={styles.author}>{m.author}</span>}
              {m.body && <span className={styles.text}>{m.body}</span>}
            </div>
          </article>
        );
      })}
    </div>
  );
}
