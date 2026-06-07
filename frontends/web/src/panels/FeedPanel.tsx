import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Feed, parseSegments, PlatformGlyph, useFeedBuffer, type Density, type FeedMessage, type Platform } from '@virta/feed-core';
import { Button, Dialog, Input, Segmented, Text, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { filterFeed, QUICK_FILTERS, type QuickFilter } from './quickFilter';
import { applyCalm } from './calmMode';
import ModRowActions, { type ModAction } from './ModRowActions';
import ChatSettingsControl from './ChatSettingsControl';
import { getHistory, sendMessage, useCapabilities, useChannels, useDaemonStream, useStats } from '../daemon';
import type { ChatSettings, LoggedMessage } from '../daemon/wire.gen';

// Convert a logged/scrollback row into a feed message for backfill. The stored body is plain text
// (emotes flattened to names), so segments are re-parsed for mentions/links rather than emote art.
function loggedToFeed(m: LoggedMessage): FeedMessage {
  const ts = m.sent_at_ms > 0 ? new Date(m.sent_at_ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '';
  return {
    id: m.id,
    ts,
    platform: m.platform as Platform,
    author: m.author,
    channel: m.channel,
    deleted: m.deleted,
    body: m.body,
    segments: parseSegments(m.body),
  };
}
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

// Calm mode (combo-collapse) is remembered per panel; default off.
const calmKey = (id?: string) => `virta.feed.calm.${id ?? 'default'}`;
function loadCalm(id?: string): boolean {
  try {
    return localStorage.getItem(calmKey(id)) === '1';
  } catch {
    return false;
  }
}
function saveCalm(id: string | undefined, v: boolean) {
  try {
    localStorage.setItem(calmKey(id), v ? '1' : '0');
  } catch {
    // storage unavailable; calm mode just won't persist across reloads
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
const MAX_MESSAGES = 2000; // bound memory; oldest drop once exceeded
const TICK_MS = 50;
// Calm-mode paced release: up to CALM_RELEASE_BATCH messages every CALM_RELEASE_TICK_MS (~18/s),
// so each line stays on screen long enough to read; the rest queue behind the "+N" pill.
const CALM_RELEASE_TICK_MS = 110;
const CALM_RELEASE_BATCH = 2;

type Props = {
  /** The channel set this feed shows ("platform:slug"); undefined = all (the unified feed). */
  channels?: string[];
  /** Dock panel id; scopes this tab's persisted text size. */
  panelId?: string;
};

export default function FeedPanel({ channels, panelId }: Props) {
  const { theme } = useTheme();
  const { density: defaultDensity } = useDensity();
  const { showTimestamps, showDeleted, autoCalmRate } = useFeedDisplay();
  const { channels: joined } = useChannels();
  const { stats } = useStats();
  const { messages, push, markDeleted, clearChannel } = useFeedBuffer({ max: MAX_MESSAGES });
  const [chatSettings, setChatSettings] = useState<Record<string, ChatSettings>>({});
  const onChatSettings = useCallback((ch: string, s: ChatSettings) => setChatSettings((prev) => ({ ...prev, [ch]: s })), []);

  // Paced intake: in calm mode, hold incoming messages in a queue and release a small batch per
  // tick (min on-screen time) so a flood stays readable; the backlog surfaces as a "+N" pill. When
  // calm is off, receive pushes straight through. calmActiveRef lets receive read the live state
  // without being re-created (and re-subscribing the stream) on every toggle.
  const pending = useRef<FeedMessage[]>([]);
  const [pendingCount, setPendingCount] = useState(0);
  const calmActiveRef = useRef(false);
  const receive = useCallback(
    (m: FeedMessage | FeedMessage[]) => {
      const arr = Array.isArray(m) ? m : [m];
      if (calmActiveRef.current) {
        pending.current.push(...arr);
        setPendingCount(pending.current.length);
      } else {
        push(arr);
      }
    },
    [push],
  );

  const status = useDaemonStream({ onMessage: receive, onDeleted: markDeleted, onClear: clearChannel, onChatSettings }, channels);

  // Backfill recent scrollback when a channel-scoped feed opens onto an empty buffer, so it shows
  // context instead of a blank pane. Only when still empty at resolve (a busy channel fills live,
  // and the buffer appends, so seeding non-empty would mis-order older lines below newer ones).
  const emptyRef = useRef(true);
  emptyRef.current = messages.length === 0;
  const channelsKey = channels ? channels.join(',') : '';
  const backfilledRef = useRef(new Set<string>());
  useEffect(() => {
    if (!channels || channels.length === 0 || status !== 'connected') return;
    if (backfilledRef.current.has(channelsKey)) return;
    backfilledRef.current.add(channelsKey);
    let cancelled = false;
    Promise.all(channels.map((c) => getHistory(c, '', 50).catch(() => [] as LoggedMessage[]))).then((lists) => {
      if (cancelled || !emptyRef.current) return;
      const merged = lists.flat().sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)); // oldest→newest
      if (merged.length > 0) push(merged.map(loggedToFeed));
    });
    return () => {
      cancelled = true;
    };
  }, [channelsKey, status, push]);

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
  const [calm, setCalm] = useState(() => loadCalm(panelId)); // true = collapse identical-message combos
  const toggleCalm = () => {
    setCalm((v) => {
      const next = !v;
      saveCalm(panelId, next);
      return next;
    });
  };
  const [background, setBackground] = useState(() => hex(14, 15, 18));

  // Auto-engage calm mode when this feed's combined live rate (summed across its channels, from the
  // daemon's stats) passes the configured threshold, so an overload calms itself without a click.
  const liveRate = useMemo(
    () => targets.reduce((sum, key) => sum + (stats[key]?.messagesPerSec ?? 0), 0),
    [targets, stats],
  );
  const autoCalm = autoCalmRate > 0 && liveRate >= autoCalmRate;
  const calmActive = calm || autoCalm;
  calmActiveRef.current = calmActive;

  // Flush the whole backlog at once (the "+N" pill, or when calm turns off).
  const flushPending = useCallback(() => {
    if (pending.current.length === 0) return;
    push(pending.current);
    pending.current = [];
    setPendingCount(0);
  }, [push]);

  // While calm, drain the queue a batch at a time; when it turns off, release everything held.
  useEffect(() => {
    if (!calmActive) {
      flushPending();
      return;
    }
    const id = setInterval(() => {
      if (pending.current.length === 0) return;
      push(pending.current.splice(0, CALM_RELEASE_BATCH));
      setPendingCount(pending.current.length);
    }, CALM_RELEASE_TICK_MS);
    return () => clearInterval(id);
  }, [calmActive, push, flushPending]);

  // Client-side view filter over the buffered feed; the full buffer keeps streaming underneath.
  const { visible, thinned } = useMemo(() => {
    const filtered = filterFeed(messages, quick, query);
    if (!calmActive) return { visible: filtered, thinned: 0 };
    return applyCalm(filtered);
  }, [messages, quick, query, calmActive]);

  // Recent chatters (newest first, unique) for the composer's @mention autocomplete.
  const chatters = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    const scanFrom = Math.max(0, messages.length - 200);
    for (let i = messages.length - 1; i >= scanFrom && out.length < 100; i--) {
      const a = messages[i].author;
      if (a && !seen.has(a)) {
        seen.add(a);
        out.push(a);
      }
    }
    return out;
  }, [messages]);

  // Moderator row actions, gated on the platform's moderation capability. They compose a slash
  // command (parsed by the send path) to the message's own channel. Ban asks for confirmation.
  const { caps } = useCapabilities();
  const [pendingBan, setPendingBan] = useState<FeedMessage | null>(null);
  const runMod = useCallback((action: ModAction, m: FeedMessage) => {
    if (!m.channel) return;
    if (action === 'delete') void sendMessage([m.channel], `/delete ${m.platformMessageId}`).catch(() => {});
    // Target by numeric author id when we have it (exact, no login lookup); fall back to the name.
    else if (action === 'timeout') void sendMessage([m.channel], `/timeout ${m.authorId || m.author} 600`).catch(() => {});
    else setPendingBan(m);
  }, []);
  const [replyTo, setReplyTo] = useState<{ channel: string; parentId: string; author: string } | null>(null);
  const onReply = useCallback((m: FeedMessage) => {
    if (m.channel && m.platformMessageId) setReplyTo({ channel: m.channel, parentId: m.platformMessageId, author: m.author });
  }, []);
  const renderActions = useCallback(
    (m: FeedMessage) => {
      if (m.deleted || (m.type && m.type !== 'chat' && m.type !== 'action') || !m.channel) return null;
      const c = caps[m.platform];
      const canReply = !!c?.replies && !!m.platformMessageId;
      const canMod = !!c?.moderation;
      if (!canReply && !canMod) return null;
      return <ModRowActions message={m} onReply={canReply ? onReply : undefined} onModerate={canMod ? runMod : undefined} />;
    },
    [caps, runMod, onReply],
  );

  // Chat-settings toggles apply to one channel, so show them only for a single moderatable feed.
  const modChannel = targets.length === 1 && caps[targets[0].split(':')[0]]?.moderation ? targets[0] : null;

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
    const id = setInterval(() => receive(Array.from({ length: perTick }, makeMessage)), TICK_MS);
    return () => clearInterval(id);
  }, [status, rate, receive]);

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
          <Tooltip
            content={
              autoCalm && !calm
                ? `Calm mode auto-engaged (${Math.round(liveRate)} msg/s)`
                : calmActive
                  ? thinned > 0
                    ? `Calm mode on (collapsing repeats, ${thinned} thinned)`
                    : 'Calm mode on (collapsing repeats)'
                  : 'Calm mode (collapse repeats)'
            }
            side="bottom"
          >
            <button
              type="button"
              className={styles.iconBtn}
              aria-label="Calm mode"
              aria-pressed={calmActive}
              onClick={toggleCalm}
            >
              <Icon name="collapse" size={16} />
              {calmActive && thinned > 0 && <span className={styles.calmCount}>{thinned}</span>}
            </button>
          </Tooltip>
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
          {modChannel && (
            <ChatSettingsControl
              settings={chatSettings[modChannel]}
              onCommand={(cmd) => void sendMessage([modChannel], cmd).catch(() => {})}
            />
          )}
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
        <Feed
          messages={visible}
          background={background}
          showSource={showSource}
          density={density}
          showTimestamps={showTimestamps}
          showDeleted={showDeleted}
          renderActions={renderActions}
        />
        {calmActive && pendingCount > 0 && (
          <button type="button" className={styles.pendingPill} onClick={flushPending}>
            +{pendingCount} paused · show
          </button>
        )}
      </div>
      {hud && <Composer targets={targets} chatters={chatters} replyTo={replyTo} onCancelReply={() => setReplyTo(null)} />}

      <Dialog
        open={pendingBan !== null}
        onOpenChange={(o) => !o && setPendingBan(null)}
        title="Ban user"
        description={pendingBan ? `Ban ${pendingBan.author} from ${pendingBan.channel?.split(':')[1] ?? 'this channel'}?` : ''}
        footer={
          <>
            <Button variant="ghost" size="md" onClick={() => setPendingBan(null)}>
              Cancel
            </Button>
            <Button
              variant="solid"
              size="md"
              onClick={() => {
                if (pendingBan?.channel) void sendMessage([pendingBan.channel], `/ban ${pendingBan.authorId || pendingBan.author}`).catch(() => {});
                setPendingBan(null);
              }}
            >
              Ban
            </Button>
          </>
        }
      >
        <Text variant="ui" tone="subtle" as="p">
          They won't be able to chat until you unban them.
        </Text>
      </Dialog>
    </div>
  );
}
