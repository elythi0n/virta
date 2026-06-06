import { useCallback, useEffect, useMemo, useState } from 'react';
import { Feed, useFeedBuffer, type FeedMessage } from '@virta/feed-core';
import { Text } from '@virta/ui-kit';
import { useDaemonStream } from '../daemon';
import { useFeedDisplay } from '../feedDisplay';
import { useTheme } from '../theme';
import styles from './MentionInbox.module.css';

const hex = (r: number, g: number, b: number) => '#' + [r, g, b].map((n) => n.toString(16).padStart(2, '0')).join('');

// A message mentions you when one of your names appears in its text (case-insensitive).
function mentionsMe(m: FeedMessage, names: string[]): boolean {
  if (names.length === 0) return false;
  const body = m.body.toLowerCase();
  return names.some((n) => body.includes(n));
}

// Collects, across every channel, the messages that mention one of your names (set in Settings).
// Subscribes to all channels and keeps only the matches, newest at the bottom like a feed.
export default function MentionInbox() {
  const { theme } = useTheme();
  const { mentionNames } = useFeedDisplay();
  const { messages, push, clear } = useFeedBuffer({ max: 500 });
  const [background, setBackground] = useState(() => hex(14, 15, 18));

  const names = useMemo(() => mentionNames.map((n) => n.toLowerCase().trim()).filter(Boolean), [mentionNames]);

  useEffect(() => {
    const value = getComputedStyle(document.documentElement).getPropertyValue('--virta-bg-0').trim();
    if (value) setBackground(value);
  }, [theme]);

  // Re-matching when the name list changes drops stale matches.
  useEffect(() => clear(), [names, clear]);

  const onMessage = useCallback(
    (m: FeedMessage) => {
      if (mentionsMe(m, names)) push(m);
    },
    [names, push],
  );
  useDaemonStream({ onMessage });

  if (names.length === 0) {
    return (
      <div className={styles.empty}>
        <Text variant="ui" tone="subtle">
          Add your names in Settings → Appearance to collect mentions here.
        </Text>
      </div>
    );
  }
  if (messages.length === 0) {
    return (
      <div className={styles.empty}>
        <Text variant="meta" tone="subtle">
          No mentions yet. Messages naming {mentionNames.join(', ')} will appear here.
        </Text>
      </div>
    );
  }
  return <Feed messages={messages} background={background} showSource density="cozy" celebrate={false} />;
}
