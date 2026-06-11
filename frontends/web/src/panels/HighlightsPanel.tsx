import { useCallback, useEffect, useState } from 'react';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import { Button, Select, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { channelKey, deleteMoment, listMoments, useChannels, useDaemonStream, type Moment } from '../daemon';
import styles from './HighlightsPanel.module.css';

const PAGE = 50;
const ALL = 'all'; // Select items can't carry an empty value, so "all channels" gets a sentinel.

const fmtTime = (iso: string) => {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
};

const durationSecs = (m: Moment) => {
  const ms = new Date(m.ended_at).getTime() - new Date(m.started_at).getTime();
  return Number.isNaN(ms) ? 0 : Math.max(0, Math.round(ms / 1000));
};

// How far above the channel's normal pace the spike got; the baseline floor keeps quiet channels
// from producing absurd multipliers.
const multiplier = (m: Moment) => m.peak_rate / Math.max(m.baseline, 0.1);

// Intensity bar fill: ×1 is empty-ish, ×16 and beyond is full.
const intensityPct = (mult: number) => Math.min(100, Math.max(6, (mult / 16) * 100));

type LoadState = 'loading' | 'ready' | 'offline';

// A newest-first timeline of auto-bookmarked chat spikes ("moments"): when chat erupts well past
// its baseline, the daemon bookmarks the burst with a short excerpt. Live moments prepend as they
// happen; older pages load on demand via the ULID cursor.
export default function HighlightsPanel() {
  const { channels } = useChannels();
  const [filter, setFilter] = useState(ALL);
  const [moments, setMoments] = useState<Moment[]>([]);
  const [state, setState] = useState<LoadState>('loading');
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);

  const channelParam = filter === ALL ? undefined : filter;

  useEffect(() => {
    let cancelled = false;
    setState('loading');
    setMoments([]);
    setHasMore(false);
    listMoments(channelParam, undefined, PAGE)
      .then((ms) => {
        if (cancelled) return;
        // Live moments may have arrived while the fetch was in flight; keep them on top.
        setMoments((prev) => {
          const seen = new Set(ms.map((m) => m.id));
          return [...prev.filter((m) => !seen.has(m.id)), ...ms];
        });
        setHasMore(ms.length >= PAGE);
        setState('ready');
      })
      .catch(() => !cancelled && setState('offline'));
    return () => {
      cancelled = true;
    };
  }, [channelParam]);

  // Live prepend. The shared stream routes moment events by the moment's channel key, and the
  // subscription below is already scoped to the active filter, so anything arriving here belongs.
  const onMoment = useCallback((_key: string, m: Moment) => {
    setMoments((prev) => (prev.some((x) => x.id === m.id) ? prev : [m, ...prev]));
  }, []);
  useDaemonStream({ onMessage: () => {}, onMoment }, channelParam ? [channelParam] : undefined);

  const loadMore = async () => {
    const last = moments[moments.length - 1];
    if (!last || loadingMore) return;
    setLoadingMore(true);
    try {
      const older = await listMoments(channelParam, last.id, PAGE);
      setMoments((prev) => {
        const seen = new Set(prev.map((m) => m.id));
        return [...prev, ...older.filter((m) => !seen.has(m.id))];
      });
      setHasMore(older.length >= PAGE);
    } catch {
      // keep the button so a transient failure can be retried
    } finally {
      setLoadingMore(false);
    }
  };

  const remove = (id: string) => {
    setMoments((prev) => prev.filter((m) => m.id !== id)); // optimistic; the daemon delete is fire-and-forget
    deleteMoment(id).catch(() => {});
  };

  const channelOptions = [
    { value: ALL, label: 'All channels' },
    ...channels.map((c) => ({ value: channelKey(c.platform, c.slug), label: `${c.platform} · ${c.slug}` })),
  ];

  return (
    <div className={styles.panel}>
      <header className={styles.head}>
        <div className={styles.headIcon}>
          <Icon name="flame" size={16} />
        </div>
        <div className={styles.headText}>
          <Text variant="title" as="h2" className={styles.title}>
            Highlights
          </Text>
          <Text variant="meta" tone="subtle" as="p" className={styles.lead}>
            Auto-bookmarked hype moments from your channels.
          </Text>
        </div>
      </header>

      <div className={styles.toolbar}>
        <Select ariaLabel="Channel filter" value={filter} onValueChange={setFilter} options={channelOptions} />
      </div>

      {state === 'offline' ? (
        <div className={styles.empty}>
          <Icon name="flame" size={28} />
          <Text variant="ui" tone="subtle">
            Not connected to the daemon.
          </Text>
        </div>
      ) : state === 'ready' && moments.length === 0 ? (
        <div className={styles.empty}>
          <Icon name="flame" size={28} />
          <Text variant="ui" tone="subtle">
            No moments yet — when chat erupts, the spike gets bookmarked here automatically.
          </Text>
        </div>
      ) : (
        <div className={styles.timeline}>
          {moments.map((m) => {
            const mult = multiplier(m);
            return (
              <article key={m.id} className={styles.card}>
                <div className={styles.cardHead}>
                  <span className={styles.time}>{fmtTime(m.started_at)}</span>
                  <span className={styles.duration}>{durationSecs(m)}s</span>
                  <span className={styles.chip}>
                    <PlatformGlyph platform={m.channel.platform as Platform} className={styles.glyph} />
                    {m.channel.slug}
                  </span>
                  <span
                    className={styles.peak}
                    title={`Peaked at ${m.peak_rate.toFixed(1)} msg/s against a ${m.baseline.toFixed(1)} msg/s baseline`}
                  >
                    ×{mult.toFixed(1)}
                    <span className={styles.peakRaw}>{m.peak_rate.toFixed(1)} msg/s</span>
                  </span>
                  <button
                    type="button"
                    className={styles.delete}
                    aria-label="Delete moment"
                    title="Delete moment"
                    onClick={() => remove(m.id)}
                  >
                    <Icon name="trash" size={14} />
                  </button>
                </div>
                <span className={styles.intensityTrack} aria-hidden>
                  <span className={styles.intensityBar} style={{ width: `${intensityPct(mult)}%` }} />
                </span>
                {m.excerpt.length > 0 && (
                  <div className={styles.excerpt}>
                    {m.excerpt.map((line, i) => (
                      <div key={i} className={styles.line}>
                        <span className={styles.author}>{line.author}</span>
                        <span className={styles.body}>{line.body}</span>
                      </div>
                    ))}
                  </div>
                )}
              </article>
            );
          })}
          {state === 'loading' && (
            <Text variant="meta" tone="subtle" className={styles.loading}>
              Loading moments…
            </Text>
          )}
          {hasMore && state === 'ready' && (
            <Button variant="ghost" size="sm" disabled={loadingMore} onClick={() => void loadMore()}>
              {loadingMore ? 'Loading…' : 'Load more'}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
