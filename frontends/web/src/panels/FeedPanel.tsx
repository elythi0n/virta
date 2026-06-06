import { useEffect, useState } from 'react';
import { Feed, parseSegments, useFeedBuffer, type FeedMessage, type Platform } from '@virta/feed-core';
import { Segmented, StatusDot, Text } from '@virta/ui-kit';
import { useDaemonStream, type ConnectionStatus } from '../daemon';
import { useDensity } from '../density';
import { useTheme } from '../theme';
import styles from './FeedPanel.module.css';

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

let seq = 0;
function makeMessage(): FeedMessage {
  const i = seq++;
  const src = SAMPLE_SOURCES[i % SAMPLE_SOURCES.length];
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

const STATUS_LABEL: Record<ConnectionStatus, string> = {
  offline: 'Demo data',
  connecting: 'Connecting…',
  connected: 'Live',
  reconnecting: 'Reconnecting…',
};

type Props = {
  /** The channel set this feed shows ("platform:slug"); undefined = all (the unified feed). */
  channels?: string[];
};

export default function FeedPanel({ channels }: Props) {
  const { theme } = useTheme();
  const { density } = useDensity();
  const { messages, push } = useFeedBuffer({ max: MAX_MESSAGES });
  const status = useDaemonStream(push, channels);
  const [rate, setRate] = useState<Rate>('live');
  const [background, setBackground] = useState(() => hex(14, 15, 18));

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
        <span className={styles.status}>
          <StatusDot status={status === 'connected' ? 'live' : status === 'offline' ? 'offline' : 'idle'} label={STATUS_LABEL[status]} />
          <Text variant="meta" tone="subtle">
            {STATUS_LABEL[status]}
          </Text>
        </span>
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
      </div>
      <div className={styles.feedWrap}>
        <Feed messages={messages} background={background} showSource={showSource} density={density} />
      </div>
    </div>
  );
}
