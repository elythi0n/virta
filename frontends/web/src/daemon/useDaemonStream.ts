import { useEffect, useRef, useState } from 'react';
import type { DeletionRef, FeedMessage } from '@virta/feed-core';
import { createDaemonClient, type ConnectionStatus } from './client';
import type { ChatSettings, HeldMessage } from './wire.gen';

export interface StreamHandlers {
  onMessage: (msg: FeedMessage) => void;
  onDeleted?: (ref: DeletionRef) => void;
  onClear?: (channelKey: string, userId?: string) => void;
  onChatSettings?: (channelKey: string, settings: ChatSettings) => void;
  onHeld?: (held: HeldMessage) => void;
  onHeldResolved?: (channelKey: string, id: string, approved: boolean) => void;
}

// Connects to the daemon for the panel's lifetime, routing live messages, deletions, and clears to
// the given handlers (typically a feed buffer's push/markDeleted/clearChannel), and reports the
// connection status. `channels` narrows the subscription to a set ("platform:slug"); empty = all.
// Handlers are read through a ref, so passing fresh closures each render never reconnects the
// socket; only a change to the channel set does.
export function useDaemonStream(handlers: StreamHandlers, channels?: string[]): ConnectionStatus {
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  const ref = useRef(handlers);
  ref.current = handlers;
  // Stable key so re-renders with an equivalent set don't reconnect.
  const key = channels ? channels.join(',') : '';
  useEffect(() => {
    const client = createDaemonClient({
      onMessage: (m) => ref.current.onMessage(m),
      onDeleted: (r) => ref.current.onDeleted?.(r),
      onClear: (c, u) => ref.current.onClear?.(c, u),
      onChatSettings: (c, s) => ref.current.onChatSettings?.(c, s),
      onHeld: (h) => ref.current.onHeld?.(h),
      onHeldResolved: (c, id, approved) => ref.current.onHeldResolved?.(c, id, approved),
      onStatus: setStatus,
      channels: key ? key.split(',') : undefined,
    });
    client.start();
    return () => client.stop();
  }, [key]);
  return status;
}
