import { useState } from 'react';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import { ContextMenu, StatusDot, Text, type ContextMenuEntry, type DotStatus } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels, useStats, useStreams } from '../daemon';
import styles from './StreamsSidebar.module.css';

function stateDot(state: string): DotStatus {
  if (state === 'connected') return 'live';
  if (state === 'error') return 'offline';
  return 'idle';
}

// Compact count: 1.2k above a thousand, whole numbers below.
const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k` : Math.round(n).toString());

type Props = {
  /** Open a feed scoped to a single channel ("platform:slug"). */
  openChannel: (channelKey: string, label: string) => void;
  /** Open a channel's embedded player. */
  openStream: (channelKey: string, label: string) => void;
  /** Existing scoped feeds a channel can be merged into. */
  listFeeds: () => { id: string; title: string }[];
  /** Merge a channel into an existing feed. */
  mergeChannelIntoFeed: (panelId: string, channelKey: string) => void;
};

// The live channel rail, Twitch-style: each joined channel as a card with a live preview
// thumbnail, a LIVE badge, and the viewer count when streaming; expand a row for the chat activity
// (msg/s, unique chatters) from the daemon's stats stream. Click the preview to watch, the name to
// open chat, or right-click for those plus merging into an existing feed.
export default function StreamsSidebar({ openChannel, openStream, listFeeds, mergeChannelIntoFeed }: Props) {
  const { channels, status } = useChannels();
  const streams = useStreams();
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
        const info = streams[key];
        const st = stats[key];
        const live = info?.live ?? false;
        const open = expanded === key;
        const feeds = listFeeds();
        const menuItems: ContextMenuEntry[] = [
          { kind: 'item', label: 'Open stream', onSelect: () => openStream(key, c.slug) },
          { kind: 'item', label: 'Open chat', onSelect: () => openChannel(key, c.slug) },
        ];
        if (feeds.length > 0) {
          menuItems.push({
            kind: 'submenu',
            label: 'Open chat into',
            items: feeds.map((f) => ({ kind: 'item', label: f.title, onSelect: () => mergeChannelIntoFeed(f.id, key) })),
          });
        }
        return (
          <li key={key} className={styles.item} data-live={live}>
            <ContextMenu
              items={menuItems}
              trigger={
                <div className={styles.card}>
            {live && (
              <button type="button" className={styles.thumb} onClick={() => openStream(key, c.slug)} aria-label={`Watch ${c.slug}`}>
                {info?.thumbnail_url ? (
                  <img className={styles.thumbImg} src={info.thumbnail_url} alt="" loading="lazy" />
                ) : (
                  <span className={styles.thumbFallback} aria-hidden />
                )}
                <span className={styles.liveBadge}>LIVE</span>
                {info && info.viewer_count > 0 && (
                  <span className={styles.viewers}>
                    <i className={styles.vdot} aria-hidden />
                    {fmt(info.viewer_count)}
                  </span>
                )}
              </button>
            )}

            <div className={styles.row}>
              <button type="button" className={styles.main} onClick={() => openChannel(key, c.slug)} title={`Open ${c.slug}`}>
                <PlatformGlyph platform={c.platform as Platform} className={styles.glyph} />
                <span className={styles.name}>{c.slug}</span>
                {!live && <span className={styles.offline}>Offline</span>}
                <StatusDot status={stateDot(c.state)} label={c.state} />
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

            {live && (info?.category || info?.title) && (
              <div className={styles.streamInfo}>
                {info?.category && <span className={styles.category}>{info.category}</span>}
                {info?.title && (
                  <span className={styles.title} title={info.title}>
                    {info.title}
                  </span>
                )}
              </div>
            )}

            {open && (
              <dl className={styles.detail}>
                <div className={styles.detailRow}>
                  <dt>Chat</dt>
                  <dd>
                    <b>{st ? fmt(st.messagesPerSec) : '—'}</b> msg/s
                  </dd>
                </div>
                <div className={styles.detailRow}>
                  <dt>Chatters</dt>
                  <dd>{st ? fmt(st.uniqueChatters) : '—'}</dd>
                </div>
                <div className={styles.detailRow}>
                  <dt>Top emote</dt>
                  <dd>{st?.topEmote ?? '—'}</dd>
                </div>
              </dl>
            )}
                </div>
              }
            />
          </li>
        );
      })}
    </ul>
  );
}
