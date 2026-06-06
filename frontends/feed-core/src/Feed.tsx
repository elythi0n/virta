import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import FeedRow, { type Density } from './FeedRow';
import { eventHeadline, eventImpact, isCelebration } from './events';
import type { FeedMessage, MessageType } from './types';
import styles from './Feed.module.css';

// Within this many px of the bottom counts as "pinned"; new messages keep the view at the latest.
const STICK_THRESHOLD = 48;
// How long a celebration banner stays up before fading out.
const BANNER_MS = 4500;

type Banner = { id: string; text: string; type: MessageType };

type FeedProps = {
  messages: FeedMessage[];
  /** Feed background (hex) author colors are contrast-clamped against; pass the theme's bg-0. */
  background: string;
  /** Show the per-row source-channel tag (for feeds aggregating multiple channels). */
  showSource?: boolean;
  /** Row density (type scale + spacing). */
  density?: Density;
  /** Show the per-row timestamp. */
  showTimestamps?: boolean;
  /** Show a deleted message's original text (struck) instead of a tombstone (mod view). */
  showDeleted?: boolean;
  /** Optional hover-revealed per-row actions (e.g. moderator buttons). */
  renderActions?: (m: FeedMessage) => ReactNode;
  /** Show the transient banner when a high-impact event (gift bomb, big raid) arrives live. */
  celebrate?: boolean;
};

// Virtualized chat feed: only the visible window is in the DOM (stable keys by message id), so
// throughput is bounded by the viewport, not the backlog. Pins to the bottom while the user is
// there; scrolling up detaches and a pill offers to jump back to the latest.
export default function Feed({ messages, background, showSource, density = 'cozy', showTimestamps = true, showDeleted = false, renderActions, celebrate = true }: FeedProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true); // live pin state, read inside the scroll handler
  const prevCount = useRef(0);
  const lastSeenId = useRef<string | null>(null); // newest id already considered for a banner
  const [atBottom, setAtBottom] = useState(true);
  const [unseen, setUnseen] = useState(0);
  const [banner, setBanner] = useState<Banner | null>(null);

  const virtualizer = useVirtualizer({
    count: messages.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 26,
    overscan: 16,
    getItemKey: (index) => messages[index].id,
  });

  const scrollToBottom = useCallback(() => {
    if (messages.length > 0) {
      virtualizer.scrollToIndex(messages.length - 1, { align: 'end' });
    }
  }, [virtualizer, messages.length]);

  // On new messages: keep pinned to the bottom, or count them as unseen while detached.
  useLayoutEffect(() => {
    const delta = messages.length - prevCount.current;
    prevCount.current = messages.length;
    if (delta <= 0) return;
    if (stick.current) {
      scrollToBottom();
    } else {
      setUnseen((u) => u + delta);
    }
  }, [messages.length, scrollToBottom]);

  // Fire a celebration banner once, when a high-impact event ARRIVES (is appended at the tail) —
  // not when its row scrolls into the virtualized viewport. The freshly-appended tail is the run
  // of messages after the last id we saw; we walk back from the newest until we hit it (bounded,
  // so a churned ring buffer never rescans deep history). The first pass only records the tail,
  // so backfilled history never celebrates on load.
  useEffect(() => {
    if (!celebrate || messages.length === 0) return;
    const prev = lastSeenId.current;
    const newest = messages[messages.length - 1].id;
    if (prev === null) {
      lastSeenId.current = newest;
      return;
    }
    if (newest === prev) return;
    lastSeenId.current = newest;
    const floor = Math.max(0, messages.length - 64);
    let top: FeedMessage | null = null;
    for (let i = messages.length - 1; i >= floor && messages[i].id !== prev; i--) {
      const m = messages[i];
      if (isCelebration(m) && (!top || eventImpact(m) > eventImpact(top))) top = m;
    }
    if (top) setBanner({ id: top.id, text: eventHeadline(top), type: top.type ?? 'system' });
  }, [messages, celebrate]);

  // Auto-dismiss the banner; a newer celebration replaces it and resets the timer.
  useEffect(() => {
    if (!banner) return;
    const t = setTimeout(() => setBanner(null), BANNER_MS);
    return () => clearTimeout(t);
  }, [banner]);

  const onScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    const bottom = distance <= STICK_THRESHOLD;
    stick.current = bottom;
    setAtBottom(bottom);
    if (bottom) setUnseen(0);
  }, []);

  const resume = useCallback(() => {
    stick.current = true;
    setUnseen(0);
    setAtBottom(true);
    scrollToBottom();
  }, [scrollToBottom]);

  const items = virtualizer.getVirtualItems();

  return (
    <div className={styles.feed}>
      <div className={styles.viewport} ref={scrollRef} onScroll={onScroll} role="log" aria-label="Chat feed">
        <div className={styles.sizer} style={{ height: virtualizer.getTotalSize() }}>
          {items.map((vi) => (
            <div
              key={vi.key}
              data-index={vi.index}
              ref={virtualizer.measureElement}
              className={styles.rowWrap}
              style={{ transform: `translateY(${vi.start}px)` }}
            >
              <FeedRow
                message={messages[vi.index]}
                background={background}
                showSource={showSource}
                density={density}
                showTimestamps={showTimestamps}
                showDeleted={showDeleted}
                renderActions={renderActions}
              />
            </div>
          ))}
        </div>
      </div>
      {banner && (
        <div className={`${styles.banner} ${!atBottom ? styles.bannerRaised : ''}`} data-type={banner.type} role="status">
          <span className={styles.bannerGlyph} aria-hidden>
            ✦
          </span>
          <span className={styles.bannerText}>{banner.text}</span>
        </div>
      )}
      {!atBottom && (
        <button className={styles.pill} onClick={resume}>
          {unseen > 0 ? `${unseen} new message${unseen === 1 ? '' : 's'}` : 'Jump to latest'} ↓
        </button>
      )}
    </div>
  );
}
