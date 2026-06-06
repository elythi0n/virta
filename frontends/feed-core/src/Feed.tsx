import { useCallback, useLayoutEffect, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import FeedRow from './FeedRow';
import type { FeedMessage } from './types';
import styles from './Feed.module.css';

// Within this many px of the bottom counts as "pinned"; new messages keep the view at the latest.
const STICK_THRESHOLD = 48;

type FeedProps = {
  messages: FeedMessage[];
  /** Feed background (hex) author colors are contrast-clamped against; pass the theme's bg-0. */
  background: string;
};

// Virtualized chat feed: only the visible window is in the DOM (stable keys by message id), so
// throughput is bounded by the viewport, not the backlog. Pins to the bottom while the user is
// there; scrolling up detaches and a pill offers to jump back to the latest.
export default function Feed({ messages, background }: FeedProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true); // live pin state, read inside the scroll handler
  const prevCount = useRef(0);
  const [atBottom, setAtBottom] = useState(true);
  const [unseen, setUnseen] = useState(0);

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
              <FeedRow message={messages[vi.index]} background={background} />
            </div>
          ))}
        </div>
      </div>
      {!atBottom && (
        <button className={styles.pill} onClick={resume}>
          {unseen > 0 ? `${unseen} new message${unseen === 1 ? '' : 's'}` : 'Jump to latest'} ↓
        </button>
      )}
    </div>
  );
}
