import { useCallback, useRef, useState } from 'react';
import { Segmented, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { useDaemonStream } from '../daemon';
import styles from './MarketsPanel.module.css';

// ── Types ──────────────────────────────────────────────────────────────────
interface Tick {
  symbol: string;
  quote: string;
  price: number;
  change_24h: number;
  change_1h?: number;
  volume_24h?: number;
  high_24h?: number;
  low_24h?: number;
  realtime: boolean;
  provider: string;
  ts: string;
}

interface ProviderStatus {
  state: 'connecting' | 'connected' | 'degraded' | 'disconnected';
  message?: string;
}

type DisplayMode = 'board' | 'ticker' | 'compact';

const STREAM_TICK   = 'plugin.com.virta.markets.tick';
const STREAM_STATUS = 'plugin.com.virta.markets.status';

// ── Sparkline ──────────────────────────────────────────────────────────────
const SPARKLINE_POINTS = 20;

function Sparkline({ history, positive }: { history: number[]; positive: boolean }) {
  if (history.length < 2) return null;
  const min = Math.min(...history);
  const max = Math.max(...history);
  const range = max - min || 1;
  const w = 60;
  const h = 24;
  const points = history.map((v, i) => {
    const x = (i / (history.length - 1)) * w;
    const y = h - ((v - min) / range) * h;
    return `${x},${y}`;
  });
  const d = `M${points.join(' L')}`;
  const color = positive ? 'var(--virta-ok)' : 'var(--virta-danger)';
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} aria-hidden className={styles.sparkline}>
      <path d={d} fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

// ── Formatters ─────────────────────────────────────────────────────────────
function fmtPrice(p: number, _quote: string): string {
  if (p >= 10_000) return `${(p / 1000).toFixed(1)}k`;
  if (p >= 1)      return p.toFixed(2);
  if (p >= 0.01)   return p.toFixed(4);
  return p.toFixed(8);
}

function fmtChange(c: number): string {
  const sign = c >= 0 ? '+' : '';
  return `${sign}${c.toFixed(2)}%`;
}

function fmtVolume(v: number): string {
  if (v >= 1_000_000_000) return `${(v / 1_000_000_000).toFixed(1)}B`;
  if (v >= 1_000_000)     return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000)         return `${(v / 1_000).toFixed(1)}K`;
  return v.toFixed(0);
}

// ── Board row ─────────────────────────────────────────────────────────────
function BoardRow({ tick, history }: { tick: Tick; history: number[] }) {
  const up = tick.change_24h >= 0;
  return (
    <div className={styles.boardRow}>
      <div className={styles.boardSymbol}>
        <span className={styles.boardBase}>{tick.symbol}</span>
        <span className={styles.boardQuote}>{tick.quote}</span>
        {!tick.realtime && <span className={styles.delayedBadge}>delayed</span>}
      </div>
      <div className={`${styles.boardChange} ${up ? styles.up : styles.down}`}>
        {fmtChange(tick.change_24h)}
      </div>
      {tick.change_1h != null && (
        <div className={`${styles.boardChange1h} ${tick.change_1h >= 0 ? styles.up : styles.down}`}>
          {fmtChange(tick.change_1h)}
        </div>
      )}
      <Sparkline history={history} positive={up} />
      <div className={styles.boardPrice}>
        {fmtPrice(tick.price, tick.quote)}
        <span className={styles.boardPriceQuote}> {tick.quote}</span>
      </div>
    </div>
  );
}

// ── Ticker item (marquee strip) ────────────────────────────────────────────
function TickerItem({ tick }: { tick: Tick }) {
  const up = tick.change_24h >= 0;
  return (
    <span className={styles.tickerItem}>
      <span className={styles.tickerSymbol}>{tick.symbol}</span>
      <span className={styles.tickerPrice}>{fmtPrice(tick.price, tick.quote)}</span>
      <span className={`${styles.tickerChange} ${up ? styles.up : styles.down}`}>
        {fmtChange(tick.change_24h)}
      </span>
    </span>
  );
}

// ── Main panel ─────────────────────────────────────────────────────────────
export default function MarketsPanel() {
  const [ticks, setTicks] = useState<Map<string, Tick>>(new Map());
  const [history, setHistory] = useState<Map<string, number[]>>(new Map());
  const [status, setStatus] = useState<ProviderStatus>({ state: 'connecting' });
  const [mode, setMode] = useState<DisplayMode>('board');
  const tickerRef = useRef<HTMLDivElement>(null);

  const onPlugin = useCallback((stream: string, data: unknown) => {
    if (stream === STREAM_TICK) {
      const t = data as Tick;
      setTicks(prev => new Map(prev).set(t.symbol, t));
      setHistory(prev => {
        const next = new Map(prev);
        const h = [...(next.get(t.symbol) ?? []), t.price].slice(-SPARKLINE_POINTS);
        next.set(t.symbol, h);
        return next;
      });
    } else if (stream === STREAM_STATUS) {
      setStatus(data as ProviderStatus);
    }
  }, []);

  useDaemonStream({ onMessage: () => {}, onPlugin });

  const sorted = [...ticks.values()].sort((a, b) =>
    (b.volume_24h ?? 0) - (a.volume_24h ?? 0),
  );

  const statusDot = {
    connected: styles.statusDotLive,
    connecting: styles.statusDotIdle,
    degraded: styles.statusDotWarn,
    disconnected: styles.statusDotOff,
  }[status.state] ?? styles.statusDotIdle;

  return (
    <div className={styles.panel}>
      {/* ── Header ── */}
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <span className={styles.statusDot + ' ' + statusDot} aria-hidden />
          <Text variant="meta" tone="subtle" className={styles.statusLabel}>
            {status.message ?? status.state}
          </Text>
        </div>
        <Segmented
          ariaLabel="Display mode"
          value={mode}
          onValueChange={v => setMode(v as DisplayMode)}
          options={[
            { value: 'board',   label: 'Board' },
            { value: 'ticker',  label: 'Ticker' },
            { value: 'compact', label: 'Compact' },
          ]}
        />
      </div>

      {/* ── Empty state ── */}
      {sorted.length === 0 && (
        <div className={styles.empty}>
          <Icon name="stats" size={28} />
          <Text variant="ui" tone="subtle">
            {status.state === 'connecting' ? 'Connecting to exchange…' : 'No price data yet.'}
          </Text>
        </div>
      )}

      {/* ── Board ── */}
      {mode === 'board' && sorted.length > 0 && (
        <div className={styles.board}>
          <div className={styles.boardHead}>
            <span>Symbol</span>
            <span>24h</span>
            <span>1h</span>
            <span></span>{/* sparkline column */}
            <span className={styles.boardPriceHead}>Price</span>
          </div>
          {sorted.map(t => (
            <BoardRow key={t.symbol} tick={t} history={history.get(t.symbol) ?? []} />
          ))}
        </div>
      )}

      {/* ── Ticker (marquee) ── */}
      {mode === 'ticker' && sorted.length > 0 && (
        <div className={styles.tickerWrap} aria-label="Price ticker">
          <div ref={tickerRef} className={styles.tickerTrack}>
            {/* Duplicate for seamless loop */}
            {[...sorted, ...sorted].map((t, i) => (
              <TickerItem key={`${t.symbol}-${i}`} tick={t} />
            ))}
          </div>
        </div>
      )}

      {/* ── Compact (dense strip) ── */}
      {mode === 'compact' && sorted.length > 0 && (
        <div className={styles.compact}>
          {sorted.map(t => {
            const up = t.change_24h >= 0;
            return (
              <div key={t.symbol} className={styles.compactItem}>
                <span className={styles.compactSymbol}>{t.symbol}</span>
                <span className={`${styles.compactChange} ${up ? styles.up : styles.down}`}>
                  {fmtChange(t.change_24h)}
                </span>
              </div>
            );
          })}
        </div>
      )}

      {/* ── Footer: volume + realtime indicator ── */}
      {sorted.length > 0 && (
        <div className={styles.footer}>
          {sorted[0] && (
            <Text variant="meta" tone="subtle">
              {sorted[0].realtime ? 'Real-time' : 'Delayed'} · {sorted[0].provider}
            </Text>
          )}
          <Text variant="meta" tone="subtle">
            Vol {sorted[0]?.volume_24h != null ? fmtVolume(sorted[0].volume_24h) : '—'} {sorted[0]?.quote}
          </Text>
        </div>
      )}
    </div>
  );
}
