import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Badge, Button, EmptyState, Input, Segmented, Text } from '@virta/ui-kit';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import Icon from '../Icon';
import { joinChannel, useChannels } from '../daemon';
import {
  searchChannels,
  topStreams,
  type DiscoveryChannel,
  type DiscoveryStream,
} from '../daemon/streamDiscovery';
import { useOpenChannel } from '../openChannel';
import styles from './DiscoveryPanel.module.css';

// Discovery: the "find streamers" surface. Two modes — Top live (sorted by viewers, paginated)
// and Search (free-text). Each row is one channel with a "Follow" button that calls the daemon
// join API (the same one Add Channel uses), so a discovery becomes a real channel in one tap.
//
// Twitch is the only platform with public top-streams / search-channels endpoints today; the
// Platform switcher still exposes Kick so the UI is forward-compatible — its panel shows a
// friendly "no public search yet, use Add Channel" hint instead of an error.

type Mode = 'top' | 'search';

interface ResultRow {
  platform: string;
  slug: string;
  displayName: string;
  title?: string;
  category?: string;
  viewerCount?: number;
  thumbnail?: string;
  isLive: boolean;
  tags?: string[];
}

const PLATFORMS = [
  { value: 'twitch', label: 'Twitch' },
  { value: 'kick', label: 'Kick' },
];

const MODES = [
  { value: 'top', label: 'Top live' },
  { value: 'search', label: 'Search' },
];

export default function DiscoveryPanel() {
  const [platform, setPlatform] = useState('twitch');
  const [mode, setMode] = useState<Mode>('top');
  const [query, setQuery] = useState('');
  const [liveOnly, setLiveOnly] = useState(true);

  const [rows, setRows] = useState<ResultRow[]>([]);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Per-row pending state so each Follow button can show its own spinner without blocking others.
  const [pending, setPending] = useState<Set<string>>(new Set());

  // Track which channels are already joined so the row can show "Following" instead of "Follow".
  const { channels: joined } = useChannels();
  const joinedSet = useMemo(
    () => new Set(joined.map((c) => `${c.platform}:${c.slug.toLowerCase()}`)),
    [joined],
  );

  // Bump on every fresh fetch so a stale response from a previous query can't overwrite the
  // current results. (Search/platform/mode change.)
  const epochRef = useRef(0);

  // Initial top-streams load + reload whenever platform or mode changes.
  useEffect(() => {
    epochRef.current += 1;
    const epoch = epochRef.current;
    setRows([]);
    setCursor(undefined);
    setError(null);
    if (mode === 'search' && !query.trim()) {
      // Search mode with no query: don't fetch — wait for input.
      return;
    }
    void runFetch(epoch, /*append*/ false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [platform, mode]);

  // Debounced search-input fetch.
  useEffect(() => {
    if (mode !== 'search') return;
    epochRef.current += 1;
    const epoch = epochRef.current;
    if (!query.trim()) {
      setRows([]);
      setCursor(undefined);
      setLoading(false);
      return;
    }
    const t = window.setTimeout(() => void runFetch(epoch, false), 280);
    return () => window.clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, liveOnly, mode]);

  const runFetch = useCallback(
    async (epoch: number, append: boolean) => {
      if (append) setLoadingMore(true);
      else setLoading(true);
      try {
        if (mode === 'top') {
          const page = await topStreams(platform, { first: 24, cursor: append ? cursor : undefined });
          if (epoch !== epochRef.current) return;
          const next = page.streams.map(streamToRow);
          setRows((prev) => (append ? [...prev, ...next] : next));
          setCursor(page.cursor || undefined);
        } else {
          const q = query.trim();
          if (!q) return;
          const results = await searchChannels(platform, q, { first: 24, liveOnly });
          if (epoch !== epochRef.current) return;
          setRows(results.map(channelToRow));
          setCursor(undefined); // search is a single page from helix
        }
        setError(null);
      } catch (err) {
        if (epoch !== epochRef.current) return;
        setError(err instanceof Error ? err.message : 'Discovery failed');
      } finally {
        if (epoch !== epochRef.current) return;
        if (append) setLoadingMore(false);
        else setLoading(false);
      }
    },
    [mode, platform, cursor, query, liveOnly],
  );

  const onLoadMore = useCallback(() => {
    if (!cursor || loadingMore) return;
    void runFetch(epochRef.current, true);
  }, [cursor, loadingMore, runFetch]);

  const openChannelPanel = useOpenChannel();

  const onFollow = useCallback(
    async (row: ResultRow) => {
      const key = `${row.platform}:${row.slug.toLowerCase()}`;
      if (joinedSet.has(key)) return;
      setPending((s) => new Set(s).add(key));
      try {
        await joinChannel(row.platform, row.slug);
      } catch (err) {
        setError(err instanceof Error ? err.message : `Couldn't follow ${row.slug}`);
      } finally {
        setPending((s) => {
          const next = new Set(s);
          next.delete(key);
          return next;
        });
      }
    },
    [joinedSet],
  );

  const onOpen = useCallback(
    (row: ResultRow) => {
      openChannelPanel(`${row.platform}:${row.slug.toLowerCase()}`, row.displayName);
    },
    [openChannelPanel],
  );

  const isKick = platform === 'kick';
  const isSearch = mode === 'search';

  return (
    <div className={styles.panel}>
      <div className={styles.toolbar}>
        <Segmented
          ariaLabel="Platform"
          value={platform}
          onValueChange={setPlatform}
          options={PLATFORMS}
        />
        <Segmented
          ariaLabel="Mode"
          value={mode}
          onValueChange={(v) => setMode(v as Mode)}
          options={MODES}
        />
      </div>

      {isSearch && (
        <div className={styles.searchBar}>
          <Input
            aria-label="Search channels"
            placeholder={isKick ? 'Kick has no public search yet — use Add Channel' : 'Search channels by name…'}
            value={query}
            onChange={(e) => setQuery(e.currentTarget.value)}
            disabled={isKick}
          />
          <Button
            variant="ghost"
            size="sm"
            aria-pressed={liveOnly}
            onClick={() => setLiveOnly((v) => !v)}
            disabled={isKick}
          >
            <Icon name="zap" size={13} />
            Live only
          </Button>
        </div>
      )}

      {error && (
        <div className={styles.error} role="alert">
          <Icon name="ban" size={14} />
          <span>{error}</span>
        </div>
      )}

      <div className={styles.body}>
        {isKick ? (
          <EmptyState
            icon={<Icon name="stream" size={28} />}
            title="No public Kick discovery yet"
            hint="Kick's public API only supports direct lookup. Use the + button in the Streams sidebar to add a Kick handle."
          />
        ) : loading ? (
          <EmptyState title="Loading…" />
        ) : rows.length === 0 ? (
          isSearch && !query.trim() ? (
            <EmptyState
              icon={<Icon name="search" size={28} />}
              title="Find streamers"
              hint="Type a name above to search Twitch channels — live or offline."
            />
          ) : (
            <EmptyState
              icon={<Icon name="stream" size={28} />}
              title={isSearch ? 'No matches' : 'No live streams right now'}
              hint={isSearch ? 'Try a different name or switch off the Live-only filter.' : 'Try again in a moment.'}
            />
          )
        ) : (
          <>
            <ul className={styles.grid}>
              {rows.map((row) => {
                const key = `${row.platform}:${row.slug.toLowerCase()}`;
                const following = joinedSet.has(key);
                const isPending = pending.has(key);
                return (
                  <li key={key} className={styles.card}>
                    <button
                      type="button"
                      className={styles.thumbBtn}
                      onClick={() => onOpen(row)}
                      aria-label={`Open ${row.displayName}`}
                    >
                      <Thumb url={row.thumbnail} />
                      {row.isLive && (
                        <span className={styles.liveBadge}>
                          <span className={styles.liveDot} />
                          LIVE
                        </span>
                      )}
                      {row.viewerCount !== undefined && row.viewerCount > 0 && (
                        <span className={styles.viewers}>{formatViewers(row.viewerCount)}</span>
                      )}
                    </button>
                    <div className={styles.cardBody}>
                      <div className={styles.cardHead}>
                        <PlatformGlyph
                          platform={row.platform as Platform}
                          className={styles.platformGlyph}
                        />
                        <span className={styles.cardName}>{row.displayName}</span>
                      </div>
                      {row.title && (
                        <Text variant="meta" tone="muted" className={styles.cardTitle}>
                          {row.title}
                        </Text>
                      )}
                      {row.category && (
                        <Badge tone="neutral" className={styles.cardCategory}>
                          {row.category}
                        </Badge>
                      )}
                      <div className={styles.cardActions}>
                        <Button
                          variant={following ? 'subtle' : 'solid'}
                          size="sm"
                          disabled={following || isPending}
                          onClick={() => onFollow(row)}
                        >
                          <Icon name={following ? 'check' : 'user-plus'} size={13} />
                          {following ? 'Following' : isPending ? 'Following…' : 'Follow'}
                        </Button>
                      </div>
                    </div>
                  </li>
                );
              })}
            </ul>
            {cursor && (
              <div className={styles.loadMore}>
                <Button variant="ghost" size="md" onClick={onLoadMore} disabled={loadingMore}>
                  {loadingMore ? 'Loading…' : 'Load more'}
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

// Thumbnail with a graceful fallback when the platform hasn't reported one yet (or hosts it
// behind a transient redirect). The CSS keeps a 16:9 aspect ratio so the grid stays uniform.
function Thumb({ url }: { url?: string }) {
  if (!url) return <div className={styles.thumbFallback} aria-hidden="true" />;
  // Helix returns templated URLs (e.g. "...{width}x{height}.jpg") — resolve to a sensible size.
  const resolved = url.replace('{width}', '320').replace('{height}', '180');
  return (
    <img
      src={resolved}
      alt=""
      className={styles.thumb}
      loading="lazy"
      onError={(e) => {
        (e.currentTarget as HTMLImageElement).style.visibility = 'hidden';
      }}
    />
  );
}

function streamToRow(s: DiscoveryStream): ResultRow {
  return {
    platform: s.platform,
    slug: s.slug,
    displayName: s.display_name,
    title: s.title,
    category: s.category,
    viewerCount: s.viewer_count,
    thumbnail: s.thumbnail,
    isLive: true,
    tags: s.tags,
  };
}

function channelToRow(c: DiscoveryChannel): ResultRow {
  return {
    platform: c.platform,
    slug: c.slug,
    displayName: c.display_name,
    title: c.title,
    category: c.category,
    thumbnail: c.thumbnail,
    isLive: c.is_live,
    tags: c.tags,
  };
}

function formatViewers(n: number): string {
  if (n < 1000) return String(n);
  if (n < 10_000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
  return Math.round(n / 1000) + 'k';
}
