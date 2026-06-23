import { useCallback, useEffect, useMemo, useState } from 'react';
import { Feed, useFeedBuffer, type FeedMessage } from '@virta/feed-core';
import { EmptyState } from '@virta/ui-kit';
import { markActivity } from '../dock/activity';
import { mentionsMe } from './mentions';
import { useDaemonStream } from '../daemon';
import { useFeedDisplay } from '../feedDisplay';
import { useTheme } from '../theme';

const hex = (r: number, g: number, b: number) => '#' + [r, g, b].map((n) => n.toString(16).padStart(2, '0')).join('');

// Collects, across every channel, the messages that mention one of your names (set in Settings).
// Subscribes to all channels and keeps only the matches, newest at the bottom like a feed.
export default function MentionInbox({ panelId }: { panelId?: string }) {
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
      if (!mentionsMe(m, names)) return;
      markActivity(panelId, 'mention');
      push(m);
    },
    [names, push, panelId],
  );
  useDaemonStream({ onMessage });

  if (names.length === 0) {
    return (
      <EmptyState
        title="No names to watch"
        hint="Add your names in Settings → Appearance to collect mentions here."
      />
    );
  }
  if (messages.length === 0) {
    return (
      <EmptyState
        title="No mentions yet"
        hint={`Messages naming ${mentionNames.join(', ')} will appear here.`}
      />
    );
  }
  return <Feed messages={messages} background={background} showSource density="cozy" celebrate={false} />;
}
