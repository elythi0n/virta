import { useMemo } from 'react';
import { PlatformGlyph, platformLabel, type Platform } from '@virta/feed-core';
import { Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels, useStats, useStreams, type ChannelStats } from '../daemon';
import styles from './StatsPanel.module.css';

const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k` : Math.round(n).toString());
// Rates under 10/s keep one decimal so slow chats don't all read "0".
const fmtRate = (n: number) => (n >= 10 ? Math.round(n).toString() : n.toFixed(1));

interface ChannelRow {
  key: string; // "platform:slug"
  platform: string;
  slug: string;
  live: boolean;
  viewers: number;
  stat?: ChannelStats;
}

interface PlatformTotals {
  platform: string;
  viewers: number;
  messagesPerSec: number;
  liveCount: number;
}

export default function StatsPanel() {
  const { channels, status } = useChannels();
  const streams = useStreams();
  const { stats } = useStats();

  const rows = useMemo<ChannelRow[]>(() => {
    const out = channels.map((c) => {
      const key = `${c.platform}:${c.slug.toLowerCase()}`;
      const info = streams[key];
      return {
        key,
        platform: c.platform,
        slug: c.slug,
        live: info?.live ?? false,
        viewers: info?.live ? info.viewer_count : 0,
        stat: stats[key],
      };
    });
    // Live first by audience, then by chat velocity so active anonymous-read channels surface too.
    out.sort((a, b) => {
      if (a.live !== b.live) return a.live ? -1 : 1;
      if (a.viewers !== b.viewers) return b.viewers - a.viewers;
      return (b.stat?.messagesPerSec ?? 0) - (a.stat?.messagesPerSec ?? 0);
    });
    return out;
  }, [channels, streams, stats]);

  const platforms = useMemo<PlatformTotals[]>(() => {
    const byPlatform = new Map<string, PlatformTotals>();
    for (const r of rows) {
      let p = byPlatform.get(r.platform);
      if (!p) {
        p = { platform: r.platform, viewers: 0, messagesPerSec: 0, liveCount: 0 };
        byPlatform.set(r.platform, p);
      }
      p.viewers += r.viewers;
      p.messagesPerSec += r.stat?.messagesPerSec ?? 0;
      if (r.live) p.liveCount++;
    }
    return [...byPlatform.values()].sort((a, b) => b.viewers - a.viewers);
  }, [rows]);

  const totals = useMemo(() => {
    let viewers = 0;
    let messagesPerSec = 0;
    let chatters = 0;
    let liveCount = 0;
    for (const r of rows) {
      viewers += r.viewers;
      messagesPerSec += r.stat?.messagesPerSec ?? 0;
      chatters += r.stat?.uniqueChatters ?? 0;
      if (r.live) liveCount++;
    }
    return { viewers, messagesPerSec, chatters, liveCount };
  }, [rows]);

  const topEmotes = useMemo(() => {
    const counts = new Map<string, number>();
    for (const r of rows) {
      for (const e of r.stat?.topEmotes ?? []) {
        counts.set(e.name, (counts.get(e.name) ?? 0) + e.count);
      }
    }
    return [...counts.entries()]
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
      .slice(0, 10);
  }, [rows]);

  const windowSeconds = rows.find((r) => r.stat)?.stat?.windowSeconds ?? 60;

  if (status === 'offline') {
    return (
      <div className={styles.panel}>
        <div className={styles.empty}>
          <Icon name="stats" size={28} />
          <Text variant="ui" tone="subtle">Not connected to the daemon.</Text>
        </div>
      </div>
    );
  }

  if (channels.length === 0) {
    return (
      <div className={styles.panel}>
        <div className={styles.empty}>
          <Icon name="stats" size={28} />
          <Text variant="ui" tone="subtle">No channels added yet.</Text>
          <Text variant="meta" tone="subtle">Stats appear once you join a channel.</Text>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.panel}>
      <div className={styles.scroll}>
        {/* ── Combined stat band ── */}
        <div className={styles.band}>
          <StatCard label="Viewers" value={fmt(totals.viewers)} hint="combined live audience" />
          <StatCard label="Msg/s" value={fmtRate(totals.messagesPerSec)} hint={`rolling ${windowSeconds}s`} />
          <StatCard
            label="Chatters"
            value={fmt(totals.chatters)}
            hint={`rolling ${windowSeconds}s`}
            title="Sum of per-channel unique chatters; someone active in two channels counts in both"
          />
          <StatCard label="Live" value={`${totals.liveCount}/${rows.length}`} hint="channels" />
        </div>

        {/* ── Per-platform breakdown ── */}
        {platforms.length > 1 && (
          <section className={styles.section} aria-label="Per-platform stats">
            <div className={styles.sectionHead}>Platforms</div>
            {platforms.map((p) => (
              <div key={p.platform} className={styles.platformRow}>
                <span className={styles.platformName}>
                  <PlatformGlyph platform={p.platform as Platform} className={styles.glyph} />
                  {platformLabel(p.platform)}
                </span>
                <span className={styles.num}>{p.viewers > 0 ? `${fmt(p.viewers)} viewers` : '—'}</span>
                <span className={styles.num}>{fmtRate(p.messagesPerSec)} msg/s</span>
              </div>
            ))}
          </section>
        )}

        {/* ── Per-channel table ── */}
        <section className={styles.section} aria-label="Per-channel stats">
          <div className={styles.sectionHead}>Channels</div>
          <div className={styles.tableHead}>
            <span>Channel</span>
            <span className={styles.num}>Viewers</span>
            <span className={styles.num}>Msg/s</span>
            <span className={styles.num}>Chatters</span>
          </div>
          {rows.map((r) => (
            <div key={r.key} className={styles.channelRow} data-live={r.live}>
              <span className={styles.channelName}>
                <i className={styles.liveDot} data-live={r.live} aria-hidden />
                <PlatformGlyph platform={r.platform as Platform} className={styles.glyph} />
                <span className={styles.slug} title={r.slug}>{r.slug}</span>
              </span>
              <span className={styles.num}>{r.live ? fmt(r.viewers) : '—'}</span>
              <span className={styles.num}>{r.stat ? fmtRate(r.stat.messagesPerSec) : '—'}</span>
              <span className={styles.num}>{r.stat ? fmt(r.stat.uniqueChatters) : '—'}</span>
            </div>
          ))}
        </section>

        {/* ── Emote leaderboard ── */}
        {topEmotes.length > 0 && (
          <section className={styles.section} aria-label="Top emotes">
            <div className={styles.sectionHead}>Top emotes</div>
            {topEmotes.map((e, i) => (
              <div key={e.name} className={styles.emoteRow}>
                <span className={styles.emoteRank}>{i + 1}</span>
                <span className={styles.emoteName} title={e.name}>{e.name}</span>
                <span className={styles.emoteBarTrack} aria-hidden>
                  <span
                    className={styles.emoteBar}
                    style={{ width: `${Math.max(4, (e.count / topEmotes[0].count) * 100)}%` }}
                  />
                </span>
                <span className={styles.num}>{fmt(e.count)}</span>
              </div>
            ))}
          </section>
        )}
      </div>

      <div className={styles.footer}>
        <Text variant="meta" tone="subtle">Live-derived · rolling {windowSeconds}s window · nothing persisted</Text>
      </div>
    </div>
  );
}

function StatCard({ label, value, hint, title }: { label: string; value: string; hint?: string; title?: string }) {
  return (
    <div className={styles.statCard} title={title}>
      <span className={styles.statLabel}>{label}</span>
      <span className={styles.statValue}>{value}</span>
      {hint && <span className={styles.statHint}>{hint}</span>}
    </div>
  );
}
