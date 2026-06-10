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

/** A deletion target: the engine's ULID (preferred) and/or the platform's own message id. */
export interface DeletionRef {
  id?: string;
  platformMessageId?: string;
}

/**
 * Mark a single deleted message (matched by engine id, else platform id). Returns the same array
 * reference when nothing matched, so React skips a re-render. Pure for unit testing.
 */
export function markDeletedIn(messages: FeedMessage[], ref: DeletionRef): FeedMessage[] {
  let changed = false;
  const next = messages.map((m) => {
    if (m.deleted) return m;
    const hit = (!!ref.id && m.id === ref.id) || (!!ref.platformMessageId && m.platformMessageId === ref.platformMessageId);
    if (!hit) return m;
    changed = true;
    return { ...m, deleted: true };
  });
  return changed ? next : messages;
}

/**
 * Mark every message a channel-clear removes: a timeout/ban (one user) or a full chat clear
 * (no user). Scoped to the cleared channel. Pure; preserves identity when nothing matched.
 */
export function clearIn(messages: FeedMessage[], channelKey: string, userId?: string): FeedMessage[] {
  let changed = false;
  const next = messages.map((m) => {
    if (m.deleted || m.channel !== channelKey) return m;
    if (userId && m.authorId !== userId) return m;
    changed = true;
    return { ...m, deleted: true };
  });
  return changed ? next : messages;
}

export interface FeedBuffer {
  messages: FeedMessage[];
  push: (incoming: FeedMessage | FeedMessage[]) => void;
  /** Strike a deleted message (moderator delete, or the author deleting their own). */
  markDeleted: (ref: DeletionRef) => void;
  /** Strike messages removed by a timeout/ban (one user) or a full channel clear (no user). */
  clearChannel: (channelKey: string, userId?: string) => void;
  clear: () => void;
}

/**
 * Buffers incoming messages and applies them on the next animation frame, so a burst of arrivals
 * (a chat firehose) becomes one render per frame instead of one render per message. The newest
 * `max` are kept to bound memory. Hidden browser tabs never fire animation frames, so while
 * hidden the queue drains on a coarse timer instead and is capped at `max` — anything past that
 * is what the capped buffer would discard at the next flush anyway.
 */
export function useFeedBuffer({ max = 2000 }: { max?: number } = {}): FeedBuffer {
  const [messages, setMessages] = useState<FeedMessage[]>([]);
  const queue = useRef<FeedMessage[]>([]);
  const frame = useRef<number | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const flush = useCallback(() => {
    frame.current = null;
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    const incoming = queue.current;
    if (incoming.length === 0) return;
    queue.current = [];
    setMessages((prev) => appendCapped(prev, incoming, max));
  }, [max]);

  const schedule = useCallback(() => {
    if (frame.current !== null || timer.current !== null) return;
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      // No frame will come while hidden; a 1 s timer keeps the queue draining (the browser may
      // throttle it further in long background stints — the queue cap absorbs that).
      timer.current = setTimeout(flush, 1000);
    } else {
      frame.current = requestAnimationFrame(flush);
    }
  }, [flush]);

  const push = useCallback(
    (incoming: FeedMessage | FeedMessage[]) => {
      if (Array.isArray(incoming)) {
        if (incoming.length === 0) return;
        queue.current = queue.current.concat(incoming);
      } else {
        queue.current.push(incoming);
      }
      if (queue.current.length > max) queue.current = queue.current.slice(queue.current.length - max);
      schedule();
    },
    [schedule, max],
  );

  // Coming back to a visible tab: replace a pending background timer with an immediate frame so
  // the backlog paints right away instead of up to a second late.
  useEffect(() => {
    if (typeof document === 'undefined') return;
    const onVisibility = () => {
      if (document.visibilityState !== 'visible' || timer.current === null) return;
      clearTimeout(timer.current);
      timer.current = null;
      if (frame.current === null) frame.current = requestAnimationFrame(flush);
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, [flush]);

  // Deletions/clears arrive well after their messages are committed (a mod action, seconds later),
  // so applying them to committed state is enough; the sub-frame window where a target is still
  // queued is not worth the StrictMode hazard of consuming the queue inside a state updater.
  const markDeleted = useCallback((ref: DeletionRef) => {
    setMessages((prev) => markDeletedIn(prev, ref));
  }, []);

  const clearChannel = useCallback((channelKey: string, userId?: string) => {
    setMessages((prev) => clearIn(prev, channelKey, userId));
  }, []);

  const clear = useCallback(() => {
    queue.current = [];
    if (frame.current !== null) {
      cancelAnimationFrame(frame.current);
      frame.current = null;
    }
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    setMessages([]);
  }, []);

  useEffect(
    () => () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
      if (timer.current !== null) clearTimeout(timer.current);
    },
    [],
  );

  return { messages, push, markDeleted, clearChannel, clear };
}
