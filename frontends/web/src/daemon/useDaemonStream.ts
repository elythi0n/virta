import { useEffect, useRef, useState } from 'react';
import type { DeletionRef, FeedMessage } from '@virta/feed-core';
import type { ConnectionStatus } from './client';
import type { ChatSettings, HeldMessage, Moment } from './wire.gen';
import { useSharedStream } from './sharedStream';

export interface StreamHandlers {
  onMessage: (msg: FeedMessage) => void;
  onDeleted?: (ref: DeletionRef) => void;
  onClear?: (channelKey: string, userId?: string) => void;
  onChatSettings?: (channelKey: string, settings: ChatSettings) => void;
  onHeld?: (held: HeldMessage) => void;
  onHeldResolved?: (channelKey: string, id: string, approved: boolean) => void;
  onPlugin?: (stream: string, data: unknown) => void;
  onMoment?: (channelKey: string, moment: Moment) => void;
}

// Connects to the daemon for the panel's lifetime, routing live messages, deletions, and clears to
// the given handlers (typically a feed buffer's push/markDeleted/clearChannel), and reports the
// connection status. `channels` narrows the subscription to a set ("platform:slug"); empty = all.
// Handlers are read through a ref, so passing fresh closures each render never reconnects the
// socket; only a change to the channel set does.
export function useDaemonStream(handlers: StreamHandlers, channels?: string[]): ConnectionStatus {
  const conn = useSharedStream();
  const [status, setStatus] = useState<ConnectionStatus>(() => conn.getStatus());
  const ref = useRef(handlers);
  ref.current = handlers;
  // Stable per-mount subscriber ID so each hook call owns its own subscription slot.
  const idRef = useRef<string | null>(null);
  if (!idRef.current) {
    idRef.current = Math.random().toString(36).slice(2);
  }
  const stableId = idRef.current;

  // Stable key so re-renders with an equivalent set don't re-subscribe.
  const key = channels ? channels.join(',') : '';

  useEffect(() => {
    const off = conn.onStatusChange(setStatus);
    setStatus(conn.getStatus());
    conn.subscribe(
      stableId,
      {
        onMessage: (m) => ref.current.onMessage(m),
        onDeleted: (r) => ref.current.onDeleted?.(r),
        onClear: (c, u) => ref.current.onClear?.(c, u),
        onChatSettings: (c, s) => ref.current.onChatSettings?.(c, s),
        onHeld: (h) => ref.current.onHeld?.(h),
        onHeldResolved: (c, id, approved) => ref.current.onHeldResolved?.(c, id, approved),
        onPlugin: (stream, data) => ref.current.onPlugin?.(stream, data),
        onMoment: (k, m) => ref.current.onMoment?.(k, m),
      },
      key ? key.split(',') : undefined,
    );
    return () => {
      conn.unsubscribe(stableId);
      off();
    };
    // Re-subscribe when the channel set changes; stableId and conn never change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  return status;
}
