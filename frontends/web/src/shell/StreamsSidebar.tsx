import { useState } from 'react';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import { StatusDot, Text, type DotStatus } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels, useStats } from '../daemon';
import styles from './StreamsSidebar.module.css';

function stateDot(state: string): DotStatus {
  if (state === 'connected') return 'live';
  if (state === 'error') return 'offline';
  return 'idle';
}
const stateLabel = (state: string) => (state === 'connected' ? 'Live' : state === 'error' ? 'Error' : 'Connecting');

// Compact count: 1.2k above a thousand, whole numbers below.
const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : Math.round(n).toString());

type Props = {
  /** Open a feed scoped to a single channel ("platform:slug"). */
  openChannel: (channelKey: string, label: string) => void;
};

// The live channel rail: each joined channel with its connection state and live activity (msg/s,
// unique chatters) from the daemon's stats stream. The daemon does not expose stream viewer counts
// yet, so we surface the chat activity it does have. Click a channel to open its own feed; expand a
// row for the fuller stats.
export default function StreamsSidebar({ openChannel }: Props) {
  const { channels, status } = useChannels();
  const { stats } = useStats();
  const [expanded, setExpanded] = useState<string | null>(null);

  if (status === 'offline') {
    return (
      <Text variant="ui" tone="subtle" as="p" className={styles.empty}>
        Not connected to a daemon. Launch the app (or run virtad) to see live streams.
      </Text>
    );
  }
  if (channels.length === 0) {
    return (
      <Text variant="meta" tone="subtle" as="p" className={styles.empty}>
        No channels joined yet. Add some in Sources.
      </Text>
    );
  }

  return (
    <ul className={styles.list}>
      {channels.map((c) => {
        const key = `${c.platform}:${c.slug.toLowerCase()}`;
        const s = stats[key];
        const open = expanded === key;
        return (
          <li key={key} className={styles.item}>
            <div className={styles.row}>
              <button type="button" className={styles.main} onClick={() => openChannel(key, c.slug)} title={`Open ${c.slug}`}>
                <PlatformGlyph platform={c.platform as Platform} className={styles.glyph} />
                <span className={styles.name}>{c.slug}</span>
                <StatusDot status={stateDot(c.state)} label={stateLabel(c.state)} />
              </button>
              <button
                type="button"
                className={styles.expand}
                aria-label={open ? `Hide ${c.slug} stats` : `Show ${c.slug} stats`}
                aria-expanded={open}
                data-open={open}
                onClick={() => setExpanded(open ? null : key)}
              >
                <Icon name="chevron-down" size={14} />
              </button>
            </div>

            <div className={styles.metrics}>
              <span className={styles.metric}>
                <b>{s ? fmt(s.messagesPerSec) : '—'}</b> msg/s
              </span>
              <span className={styles.metric}>
                <b>{s ? fmt(s.uniqueChatters) : '—'}</b> chatters
              </span>
            </div>

            {open && (
              <dl className={styles.detail}>
                <div className={styles.detailRow}>
                  <dt>Top emote</dt>
                  <dd>{s?.topEmote ?? '—'}</dd>
                </div>
                <div className={styles.detailRow}>
                  <dt>Window</dt>
                  <dd>{s ? `${s.windowSeconds}s` : '—'}</dd>
                </div>
                {c.reason && (
                  <div className={styles.detailRow}>
                    <dt>State</dt>
                    <dd>{c.reason}</dd>
                  </div>
                )}
              </dl>
            )}
          </li>
        );
      })}
    </ul>
  );
}
