import { useEffect, useState } from 'react';
import type { FeedMessage } from '@virta/feed-core';
import { createDaemonClient, type ConnectionStatus } from './client';

// Connects to the daemon for the panel's lifetime, pushing each live message to `onMessage` (a
// stable callback, e.g. a feed buffer's push), and reports the connection status for the UI.
export function useDaemonStream(onMessage: (msg: FeedMessage) => void): ConnectionStatus {
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  useEffect(() => {
    const client = createDaemonClient({ onMessage, onStatus: setStatus });
    client.start();
    return () => client.stop();
  }, [onMessage]);
  return status;
}
