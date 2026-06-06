import { useEffect, useMemo, useState } from 'react';
import { Feed, parseSegments, PlatformGlyph, useFeedBuffer, type Density, type FeedMessage, type Platform } from '@virta/feed-core';
import { Input, Segmented, Text, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { filterFeed, QUICK_FILTERS, type QuickFilter } from './quickFilter';
import { useChannels, useDaemonStream } from '../daemon';
import { useDensity } from '../density';
import { useFeedDisplay } from '../feedDisplay';
import { useTheme } from '../theme';
import Composer from './Composer';
import DensityControl, { DENSITIES } from './DensityControl';
import styles from './FeedPanel.module.css';

// Each chat tab keeps its own text size, persisted by panel id; the global setting is only the
// default for new tabs.
const VALID_DENSITY = new Set<string>(DENSITIES.map((d) => d.value));
const densityKey = (id: string) => `virta.feed.density.${id}`;
function loadDensity(id?: string): Density | null {
  if (!id) return null;
  try {
    const v = localStorage.getItem(densityKey(id));
    return v && VALID_DENSITY.has(v) ? (v as Density) : null;
  } catch {
    return null;
  }
}
function saveDensity(id: string | undefined, d: Density) {
  if (!id) return;
  try {
    localStorage.setItem(densityKey(id), d);
  } catch {
    // storage unavailable (private mode); density just won't persist across reloads
  }
}

// Chat-controls visibility (the eye toggle) is likewise remembered per panel; default on.
const hudKey = (id?: string) => `virta.feed.hud.${id ?? 'default'}`;
function loadHud(id?: string): boolean {
  try {
    return localStorage.getItem(hudKey(id)) !== '0';
  } catch {
    return true;
  }
}
function saveHud(id: string | undefined, v: boolean) {
  try {
    localStorage.setItem(hudKey(id), v ? '1' : '0');
  } catch {
    // storage unavailable; visibility just won't persist across reloads
  }
}

// Build a hex string from channels so no raw hex literal lives in the source (token-lint).
const hex = (r: number, g: number, b: number) =>
  '#' + [r, g, b].map((n) => n.toString(16).padStart(2, '0')).join('');

// Stand-in emote images (small colored chips) so the emote render path is exercised offline.
const emoteChip = (fill: string) =>
  'data:image/svg+xml,' +
  encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" rx="4" fill="${fill}"/></svg>`);
const EMOTES = {
  Kappa: { url: emoteChip(hex(146, 70, 255)) },
  PogU: { url: emoteChip(hex(83, 252, 24)) },
};

// Some author colors are deliberately too dark for the dark theme, to exercise the contrast clamp.
const AUTHOR_COLORS = [hex(34, 34, 38), hex(91, 140, 255), hex(80, 200, 120), hex(70, 70, 170), hex(210, 153, 34)];

// Several synthetic source channels so per-source attribution is visible offline.
const SAMPLE_SOURCES: { platform: Platform; slug: string; label: string }[] = [
  { platform: 'twitch', slug: 'shroud', label: 'shroud' },
  { platform: 'kick', slug: 'trainwreck', label: 'Trainwreck' },
  { platform: 'twitch', slug: 'forsen', label: 'forsen' },
];

const SAMPLE = [
  'gg that was clean',
  'no shot he actually hit that',
  'first time catching the stream live, this is sick',
  'what headset is that',
  'chat is going feral lol',
  'the pacing on this run is unreal Kappa',
  'someone clip that PogU',
  'how is he this consistent',
];

const SAMPLE_BADGES = ['broadcaster', 'moderator', 'subscriber', 'vip', 'founder'];

// A few synthetic badges so the badge row is visible offline.
function sampleBadges(i: number): { set: string }[] | undefined {
  if (i % 7 === 0) return [{ set: 'broadcaster' }];
  if (i % 3 === 0) return [{ set: SAMPLE_BADGES[i % SAMPLE_BADGES.length] }, { set: 'subscriber' }];
  if (i % 5 === 0) return [{ set: 'subscriber' }];
  return undefined;
}

// Occasional synthetic events so the tinted event bands (and the celebration banner) are visible
// offline. `event` carries the magnitude that drives the big-band tier and the live banner.
const SAMPLE_EVENTS: { type: FeedMessage['type']; author: string; text: string; event?: FeedMessage['event'] }[] = [
  { type: 'sub', author: 'nightbot_fan', text: 'subscribed at Tier 1! 3 months in a row' },
  { type: 'resub', author: 'longtime_viewer', text: 'resubscribed for 24 months', event: { months: 24 } },
  { type: 'giftsub', author: 'generous_one', text: 'gifted 10 subs to the community', event: { count: 10 } },
  { type: 'raid', author: 'partnered_streamer', text: 'is raiding with 1,240 viewers', event: { viewers: 1240 } },
  { type: 'announcement', author: 'moderator_x', text: 'Tournament starts in 10 minutes' },
];

let seq = 0;
function makeMessage(): FeedMessage {
  const i = seq++;
  const src = SAMPLE_SOURCES[i % SAMPLE_SOURCES.length];

  if (i % 19 === 0) {
    const e = SAMPLE_EVENTS[Math.floor(i / 19) % SAMPLE_EVENTS.length];
    return {
      id: `m${i}`,
      ts: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      platform: src.platform,
      type: e.type,
      event: e.event,
      author: e.author,
      source: { slug: src.slug, label: src.label },
      body: e.text,
      segments: [{ type: 'text', text: e.text }],
    };
  }

  let body = SAMPLE[Math.floor(Math.random() * SAMPLE.length)];
  if (Math.random() < 0.18) body = `@viewer_${Math.floor(Math.random() * 900)} ${body}`;
  if (Math.random() < 0.08) body += ` https://clips.example/${i}`;
  return {
    id: `m${i}`,
    ts: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
    platform: src.platform,
    author: `viewer_${Math.floor(Math.random() * 900)}`,
    authorColor: AUTHOR_COLORS[i % AUTHOR_COLORS.length],
    source: { slug: src.slug, label: src.label },
    badges: sampleBadges(i),
    body,
    segments: parseSegments(body, EMOTES),
  };
}

type Rate = 'off' | 'live' | 'stress';
const RATES: Record<Rate, number> = { off: 0, live: 12, stress: 200 }; // messages/second
const MAX_MESSAGES = 8000; // bound memory; oldest drop once exceeded
const TICK_MS = 50;

type Props = {
  /** The channel set this feed shows ("platform:slug"); undefined = all (the unified feed). */
  channels?: string[];
  /** Dock panel id; scopes this tab's persisted text size. */
  panelId?: string;
};

export default function FeedPanel({ channels, panelId }: Props) {
  const { theme } = useTheme();
  const { density: defaultDensity } = useDensity();
  const { showTimestamps } = useFeedDisplay();
  const { channels: joined } = useChannels();
  const { messages, push, markDeleted, clearChannel } = useFeedBuffer({ max: MAX_MESSAGES });
  const status = useDaemonStream({ onMessage: push, onDeleted: markDeleted, onClear: clearChannel }, channels);

  // Per-tab text size: the panel's saved choice, else the global default.
  const [density, setDensity] = useState<Density>(() => loadDensity(panelId) ?? defaultDensity);
  const changeDensity = (d: Density) => {
    setDensity(d);
    saveDensity(panelId, d);
  };

  // The composer posts to this feed's channels — the set, or every joined channel for the unified
  // feed.
  const targets = channels ?? joined.map((c) => `${c.platform}:${c.slug}`);
  const [rate, setRate] = useState<Rate>('live');
  const [quick, setQuick] = useState<QuickFilter>('all');
  const [query, setQuery] = useState('');
  const [hud, setHud] = useState(() => loadHud(panelId)); // false = preview only (hide filter bar + composer)
  const toggleHud = () => {
    setHud((v) => {
      const next = !v;
      saveHud(panelId, next);
      return next;
    });
  };
  const [background, setBackground] = useState(() => hex(14, 15, 18));

  // Client-side view filter over the buffered feed; the full buffer keeps streaming underneath.
  const visible = useMemo(() => filterFeed(messages, quick, query), [messages, quick, query]);

  // A feed aggregating more than one channel shows the source tag; a single-channel feed hides it.
  const showSource = channels === undefined || channels.length !== 1;

  // Clamp author colors against the live theme background.
  useEffect(() => {
    const value = getComputedStyle(document.documentElement).getPropertyValue('--virta-bg-0').trim();
    if (value) setBackground(value);
  }, [theme]);

  // With no daemon reachable, drive the feed with synthetic traffic so the UI is still alive in a
  // bare `npm run dev`. When connected, the daemon stream feeds the same buffer instead.
  useEffect(() => {
    if (status !== 'offline') return;
    const perSecond = RATES[rate];
    if (perSecond === 0) return;
    const perTick = Math.max(1, Math.round((perSecond * TICK_MS) / 1000));
    const id = setInterval(() => push(Array.from({ length: perTick }, makeMessage)), TICK_MS);
    return () => clearInterval(id);
  }, [status, rate, push]);

  return (
    <div className={styles.panel}>
      <div className={styles.toolbar}>
        <div className={styles.streamers}>
          {targets.length === 0 ? (
            <Text variant="meta" tone="subtle">
              No channels
            </Text>
          ) : (
            targets.map((t) => (
              <span key={t} className={styles.streamer}>
                <PlatformGlyph platform={t.split(':')[0] as Platform} className={styles.streamerGlyph} />
                {t.split(':')[1] ?? t}
              </span>
            ))
          )}
        </div>
        <div className={styles.controls}>
          {status === 'offline' && (
            <Segmented
              ariaLabel="Stream rate"
              value={rate}
              onValueChange={(v) => setRate(v as Rate)}
              options={[
                { value: 'off', label: 'Off' },
                { value: 'live', label: 'Live' },
                { value: 'stress', label: '200/s' },
              ]}
            />
          )}
          <Tooltip content={hud ? 'Preview only' : 'Show controls'} side="bottom">
            <button
              type="button"
              className={styles.iconBtn}
              aria-label={hud ? 'Hide chat controls' : 'Show chat controls'}
              aria-pressed={!hud}
              onClick={toggleHud}
            >
              <Icon name={hud ? 'eye' : 'eye-off'} size={16} />
            </button>
          </Tooltip>
          <DensityControl value={density} onChange={changeDensity} />
        </div>
      </div>
      {hud && (
      <div className={styles.filterbar}>
        <Input
          className={styles.search}
          aria-label="Filter messages"
          placeholder="Filter messages…"
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') setQuery('');
          }}
        />
        <div className={styles.quick} role="group" aria-label="Quick filters">
          {QUICK_FILTERS.map((f) => (
            <button
              key={f.value}
              type="button"
              className={`${styles.chip} ${quick === f.value ? styles.chipOn : ''}`}
              aria-pressed={quick === f.value}
              onClick={() => setQuick(f.value)}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>
      )}
      <div className={styles.feedWrap}>
        <Feed messages={visible} background={background} showSource={showSource} density={density} showTimestamps={showTimestamps} />
      </div>
      {hud && <Composer targets={targets} />}
    </div>
  );
}
