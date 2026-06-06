import { useCallback, useEffect, useRef, useState } from 'react';
import type { FeedMessage } from './types';

/**
 * Append `incoming` to `prev`, keeping only the newest `max`. Pure, so the cap/ordering is
 * unit-testable without a frame loop.
 */
export function appendCapped(prev: FeedMessage[], incoming: FeedMessage[], max: number): FeedMessage[] {
  if (incoming.length === 0) return prev;
  const combined = prev.length === 0 ? incoming.slice() : prev.concat(incoming);
  return combined.length > max ? combined.slice(combined.length - max) : combined;
}

export interface FeedBuffer {
  messages: FeedMessage[];
  push: (incoming: FeedMessage | FeedMessage[]) => void;
  clear: () => void;
}

/**
 * Buffers incoming messages and applies them on the next animation frame, so a burst of arrivals
 * (a chat firehose) becomes one render per frame instead of one render per message. The newest
 * `max` are kept to bound memory.
 */
export function useFeedBuffer({ max = 5000 }: { max?: number } = {}): FeedBuffer {
  const [messages, setMessages] = useState<FeedMessage[]>([]);
  const queue = useRef<FeedMessage[]>([]);
  const frame = useRef<number | null>(null);

  const flush = useCallback(() => {
    frame.current = null;
    const incoming = queue.current;
    if (incoming.length === 0) return;
    queue.current = [];
    setMessages((prev) => appendCapped(prev, incoming, max));
  }, [max]);

  const push = useCallback(
    (incoming: FeedMessage | FeedMessage[]) => {
      if (Array.isArray(incoming)) {
        if (incoming.length === 0) return;
        queue.current = queue.current.concat(incoming);
      } else {
        queue.current.push(incoming);
      }
      if (frame.current === null) frame.current = requestAnimationFrame(flush);
    },
    [flush],
  );

  const clear = useCallback(() => {
    queue.current = [];
    if (frame.current !== null) {
      cancelAnimationFrame(frame.current);
      frame.current = null;
    }
    setMessages([]);
  }, []);

  useEffect(
    () => () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
    },
    [],
  );

  return { messages, push, clear };
}
