import { useState } from 'react';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import { ContextMenu, StatusDot, Text, type ContextMenuEntry, type DotStatus } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels, useStats, useStreams } from '../daemon';
import { groupStreams, type StreamGroup } from './streamGroups';
import styles from './StreamsSidebar.module.css';

function stateDot(state: string): DotStatus {
  // Engine health states: "ok" = connected, "degraded" = partial, "down" = failed.
  // "connecting" is emitted by the hosted-mode channel list for channels not yet in the engine.
  if (state === 'ok') return 'live';
  if (state === 'down' || state === 'error') return 'offline';
  return 'idle'; // degraded, connecting, unknown
}

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
// Compact count: 1.2k above a thousand, whole numbers below.
const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k` : Math.round(n).toString());

/** How the rail draws each streamer: large preview cards, or a compact one-line list. */
export type StreamLayout = 'cards' | 'list';

type Props = {
  /** Card previews vs a compact list. */
  layout: StreamLayout;
  /** Open a feed scoped to a single channel ("platform:slug"). */
  openChannel: (channelKey: string, label: string) => void;
  /** Open a channel's embedded player. */
  openStream: (channelKey: string, label: string) => void;
  /** Existing scoped feeds a channel can be merged into. */
  listFeeds: () => { id: string; title: string }[];
  /** Merge a channel into an existing feed. */
  mergeChannelIntoFeed: (panelId: string, channelKey: string) => void;
};

// The live channel rail, Twitch-style. Channels that share a name across platforms collapse into
// one streamer card surfacing the best platform (Twitch first, since its thumbnail + viewer count
// come through), sorted with the most-watched live streamers on top. Click the preview to watch or
// the name to open chat (the primary platform); right-click for those plus opening a specific
// platform or merging chat into an existing feed.
export default function StreamsSidebar({ layout, openChannel, openStream, listFeeds, mergeChannelIntoFeed }: Props) {
  const { channels, status, leave } = useChannels();
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
        No channels joined yet. Add one with the + above.
      </Text>
    );
  }

  const groups = groupStreams(channels, streams);
  const compact = layout === 'list';

  return (
    <ul className={styles.list} data-layout={layout}>
      {groups.map((g) => (
        <StreamCard
          key={g.name}
          group={g}
          compact={compact}
          stat={stats[g.primary.key]}
          open={expanded === g.name}
          onToggle={() => setExpanded(expanded === g.name ? null : g.name)}
          openChannel={openChannel}
          openStream={openStream}
          feeds={listFeeds()}
          mergeChannelIntoFeed={mergeChannelIntoFeed}
          onRemove={() => g.variants.forEach((v) => void leave(v.platform, v.slug))}
        />
      ))}
    </ul>
  );
}

function StreamCard({
  group,
  compact,
  stat,
  open,
  onToggle,
  openChannel,
  openStream,
  feeds,
  mergeChannelIntoFeed,
  onRemove,
}: {
  group: StreamGroup;
  compact: boolean;
  stat?: { messagesPerSec: number; uniqueChatters: number; topEmote?: string };
  open: boolean;
  onToggle: () => void;
  openChannel: (key: string, label: string) => void;
  openStream: (key: string, label: string) => void;
  feeds: { id: string; title: string }[];
  mergeChannelIntoFeed: (panelId: string, channelKey: string) => void;
  onRemove: () => void;
}) {
  const { primary, display, variants, live, viewers } = group;
  const info = primary.info;
  const liveCount = variants.filter((v) => v.info?.live).length;

  const menuItems: ContextMenuEntry[] = [
    { kind: 'item', label: 'Open stream', onSelect: () => openStream(primary.key, display) },
    { kind: 'item', label: 'Open chat', onSelect: () => openChannel(primary.key, display) },
  ];
  // Each additional platform this streamer is on gets its own "Open on …" submenu, so a Kick
  // stream (or any other) stays reachable even though Twitch leads the card.
  for (const v of variants) {
    if (v.key === primary.key) continue;
    menuItems.push({
      kind: 'submenu',
      label: `Open on ${cap(v.platform)}`,
      items: [
        { kind: 'item', label: 'Stream', onSelect: () => openStream(v.key, display) },
        { kind: 'item', label: 'Chat', onSelect: () => openChannel(v.key, display) },
      ],
    });
  }
  if (feeds.length > 0) {
    menuItems.push({
      kind: 'submenu',
      label: 'Open chat into',
      items: feeds.map((f) => ({ kind: 'item', label: f.title, onSelect: () => mergeChannelIntoFeed(f.id, primary.key) })),
    });
  }
  menuItems.push({ kind: 'separator' });
  menuItems.push({
    kind: 'item',
    label: variants.length > 1 ? 'Remove from streams (all platforms)' : 'Remove from streams',
    danger: true,
    onSelect: onRemove,
  });

  return (
    <li className={styles.item} data-live={live}>
      <ContextMenu
        items={menuItems}
        trigger={
          <div className={styles.card}>
            {!compact && live && (
              <button type="button" className={styles.thumb} onClick={() => openStream(primary.key, display)} aria-label={`Watch ${display}`}>
                {info?.thumbnail_url ? (
                  <img className={styles.thumbImg} src={info.thumbnail_url} alt="" loading="lazy" />
                ) : (
                  <span className={styles.thumbFallback} aria-hidden />
                )}
                <span className={styles.liveBadge}>LIVE</span>
                {viewers > 0 && (
                  <span className={styles.viewers} title={liveCount > 1 ? `${fmt(viewers)} across ${liveCount} platforms` : undefined}>
                    <i className={styles.vdot} aria-hidden />
                    {fmt(viewers)}
                    {liveCount > 1 && <span className={styles.viewersAll} aria-hidden> all</span>}
                  </span>
                )}
              </button>
            )}

            <div className={styles.row}>
              <button type="button" className={styles.main} onClick={() => openChannel(primary.key, display)} title={`Open ${display}`}>
                <span className={styles.glyphs}>
                  {variants.map((v) => (
                    <PlatformGlyph
                      key={v.key}
                      platform={v.platform as Platform}
                      className={v.key === primary.key ? styles.glyph : styles.glyphMuted}
                    />
                  ))}
                </span>
                <span className={styles.name}>{display}</span>
                {compact && live && (
                  <span
                    className={styles.inlineLive}
                    title={liveCount > 1 ? `${fmt(viewers)} across ${liveCount} platforms` : undefined}
                  >
                    <i className={styles.vdot} aria-hidden />
                    {viewers > 0 ? fmt(viewers) : 'LIVE'}
                  </span>
                )}
                {!live && <span className={styles.offline}>Offline</span>}
                <StatusDot status={stateDot(primary.state)} label={primary.state} />
              </button>
              <button
                type="button"
                className={styles.expand}
                aria-label={open ? `Hide ${display} stats` : `Show ${display} stats`}
                aria-expanded={open}
                data-open={open}
                onClick={onToggle}
              >
                <Icon name="chevron-down" size={14} />
              </button>
            </div>

            {!compact && live && (info?.category || info?.title) && (
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
                    <b>{stat ? fmt(stat.messagesPerSec) : '—'}</b> msg/s
                  </dd>
                </div>
                <div className={styles.detailRow}>
                  <dt>Chatters</dt>
                  <dd>{stat ? fmt(stat.uniqueChatters) : '—'}</dd>
                </div>
                <div className={styles.detailRow}>
                  <dt>Top emote</dt>
                  <dd>{stat?.topEmote ?? '—'}</dd>
                </div>
              </dl>
            )}
          </div>
        }
      />
    </li>
  );
}
