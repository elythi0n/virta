import { useState, useMemo } from 'react';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import { ContextMenu, Text, type ContextMenuEntry } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels, useStats, useStreams } from '../daemon';
import { groupStreams, type StreamGroup } from '../shell/streamGroups';
import { useOpenChannel } from '../openChannel';
import { useOpenStream } from '../openStream';
import { useOpenUnifiedChat } from '../openUnifiedChat';
import styles from './StreamPane.module.css';

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k` : Math.round(n).toString());

export default function StreamPane() {
  const { channels, status, leave } = useChannels();
  const streams = useStreams();
  const { stats } = useStats();
  const openChannel = useOpenChannel();
  const openStream = useOpenStream();
  const openUnifiedChat = useOpenUnifiedChat();
  const [query, setQuery] = useState('');

  const groups = useMemo(() => {
    const all = groupStreams(channels, streams);
    const q = query.trim().toLowerCase();
    if (!q) return all;
    return all.filter(g => g.name.toLowerCase().includes(q) || g.display.toLowerCase().includes(q));
  }, [channels, streams, query]);

  if (status === 'offline') {
    return (
      <div className={styles.pane}>
        <div className={styles.empty}>
          <Icon name="stream" size={28} />
          <Text variant="ui" tone="subtle">Not connected to the daemon.</Text>
        </div>
      </div>
    );
  }

  const listFeeds = (): { id: string; title: string }[] => [];

  return (
    <div className={styles.pane}>
      <div className={styles.toolbar}>
        <div className={styles.search}>
          <Icon name="search" size={14} className={styles.searchIcon} />
          <input
            className={styles.searchInput}
            type="search"
            placeholder="Search streams…"
            value={query}
            onChange={e => setQuery(e.target.value)}
            aria-label="Search streams"
          />
          {query && (
            <button type="button" className={styles.searchClear} onClick={() => setQuery('')} aria-label="Clear search">
              <Icon name="x" size={12} />
            </button>
          )}
        </div>
      </div>

      {channels.length === 0 ? (
        <div className={styles.empty}>
          <Icon name="stream" size={28} />
          <Text variant="ui" tone="subtle">No channels added yet.</Text>
          <Text variant="meta" tone="subtle">Add a channel with the + button in the sidebar.</Text>
        </div>
      ) : groups.length === 0 ? (
        <div className={styles.empty}>
          <Text variant="ui" tone="subtle">No streams match "{query}".</Text>
        </div>
      ) : (
        <ul className={styles.grid} role="list">
          {groups.map(g => (
            <StreamCard
              key={g.name}
              group={g}
              stat={stats[g.primary.key]}
              feeds={listFeeds()}
              openChannel={openChannel}
              openStream={openStream}
              openUnifiedChat={openUnifiedChat}
              onRemove={() => g.variants.forEach(v => void leave(v.platform, v.slug))}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function StreamCard({
  group,
  stat,
  feeds,
  openChannel,
  openStream,
  openUnifiedChat,
  onRemove,
}: {
  group: StreamGroup;
  stat?: { messagesPerSec: number; uniqueChatters: number; topEmote?: string };
  feeds: { id: string; title: string }[];
  openChannel: (key: string, label: string) => void;
  openStream: (key: string, label: string) => void;
  openUnifiedChat: (keys: string[], label: string) => void;
  onRemove: () => void;
}) {
  const { primary, display, variants, live, viewers } = group;
  const info = primary.info;
  const liveCount = variants.filter(v => v.info?.live).length;
  const isMultiPlatform = variants.length > 1;
  const allKeys = variants.map(v => v.key);

  const menuItems: ContextMenuEntry[] = [
    { kind: 'item', label: 'Watch stream', onSelect: () => openStream(primary.key, display) },
    {
      kind: 'item',
      label: isMultiPlatform ? 'Open unified chat' : 'Open chat',
      onSelect: () => openUnifiedChat(allKeys, display),
    },
  ];
  if (isMultiPlatform) {
    for (const v of variants) {
      menuItems.push({
        kind: 'item',
        label: `${cap(v.platform)} chat only`,
        onSelect: () => openChannel(v.key, display),
      });
    }
  }
  if (feeds.length > 0) {
    menuItems.push({
      kind: 'submenu',
      label: 'Open chat into',
      items: feeds.map(f => ({ kind: 'item', label: f.title, onSelect: () => {} })),
    });
  }
  menuItems.push({ kind: 'separator' });
  menuItems.push({
    kind: 'item',
    label: isMultiPlatform ? 'Remove from streams (all platforms)' : 'Remove from streams',
    danger: true,
    onSelect: onRemove,
  });

  return (
    <li
      className={styles.item}
      data-live={live}
      draggable
      onDragStart={(e) => {
        e.dataTransfer.effectAllowed = 'copy';
        e.dataTransfer.setData('virta/panel', JSON.stringify({ kind: 'feed', channels: [primary.key], title: display }));
      }}
    >
      <ContextMenu items={menuItems} trigger={
        <div className={styles.card}>
          {/* Thumbnail / offline placeholder */}
          {live ? (
            <button
              type="button"
              className={styles.thumb}
              onClick={() => openStream(primary.key, display)}
              aria-label={`Watch ${display}`}
            >
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
          ) : (
            <button
              type="button"
              className={styles.thumbOfflineBtn}
              onClick={() => openUnifiedChat(allKeys, display)}
              aria-label={`Open chat for ${display}`}
            >
              <span className={styles.thumbOfflinePattern} aria-hidden />
              <span className={styles.offlineBadge}>OFFLINE</span>
            </button>
          )}

          {/* Info section */}
          <div className={styles.info}>
            {/* Name row */}
            <div className={styles.row}>
              <button
                type="button"
                className={styles.nameBtn}
                onClick={() => openUnifiedChat(allKeys, display)}
                title={isMultiPlatform ? `Open unified chat for ${display}` : `Open chat for ${display}`}
              >
                {display}
              </button>
              <button
                type="button"
                className={styles.chatBtn}
                onClick={() => openUnifiedChat(allKeys, display)}
                aria-label={isMultiPlatform ? `Open unified chat for ${display}` : `Open chat for ${display}`}
                title={isMultiPlatform ? 'Open unified chat' : 'Open chat'}
              >
                <Icon name="chat" size={13} />
              </button>
            </div>

            {/* Per-platform chips — only shown when the streamer is on multiple platforms */}
            {isMultiPlatform && (
              <div className={styles.platformChips}>
                {variants.map(v => (
                  <button
                    key={v.key}
                    type="button"
                    className={styles.platformChip}
                    onClick={() => openChannel(v.key, display)}
                    title={`Open ${cap(v.platform)} chat only`}
                  >
                    <PlatformGlyph platform={v.platform as Platform} className={styles.chipGlyph} />
                    <span>{cap(v.platform)}</span>
                    <span className={styles.chipArrow}>›</span>
                  </button>
                ))}
              </div>
            )}

            {live && (info?.category || info?.title) && (
              <div className={styles.meta}>
                {info?.category && <span className={styles.category}>{info.category}</span>}
                {info?.title && <span className={styles.title} title={info.title}>{info.title}</span>}
              </div>
            )}
            {live && stat && (
              <div className={styles.chatStat}>
                <Icon name="chat" size={11} />
                <span>{fmt(stat.messagesPerSec)} msg/s</span>
                {stat.topEmote && <span className={styles.topEmote}>{stat.topEmote}</span>}
              </div>
            )}
          </div>
        </div>
      } />
    </li>
  );
}
