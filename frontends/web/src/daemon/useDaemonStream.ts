import { useEffect, useState } from 'react';
import type { FeedMessage } from '@virta/feed-core';
import { createDaemonClient, type ConnectionStatus } from './client';

// Connects to the daemon for the panel's lifetime, pushing each live message to `onMessage` (a
// stable callback, e.g. a feed buffer's push), and reports the connection status for the UI.
// `channels` narrows the subscription to a set ("platform:slug"); empty/undefined = all channels.
export function useDaemonStream(onMessage: (msg: FeedMessage) => void, channels?: string[]): ConnectionStatus {
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  // Stable key so re-renders with an equivalent set don't reconnect.
  const key = channels ? channels.join(',') : '';
  useEffect(() => {
    const client = createDaemonClient({
      onMessage,
      onStatus: setStatus,
      channels: key ? key.split(',') : undefined,
    });
    client.start();
    return () => client.stop();
  }, [onMessage, key]);
  return status;
}
