import { useCallback, useEffect, useState } from 'react';
import { DaemonUnreachableError, joinChannel, leaveChannel, listChannels } from './api';
import type { ChannelInfo } from './wire.gen';

export type ChannelsStatus = 'loading' | 'ready' | 'offline';

// Loads the daemon's joined channels and exposes join/leave that refresh the list. `offline` means
// no daemon is reachable (e.g. a bare web dev server); the UI shows a hint rather than an error.
export function useChannels() {
  const [channels, setChannels] = useState<ChannelInfo[]>([]);
  const [status, setStatus] = useState<ChannelsStatus>('loading');
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setChannels(await listChannels());
      setStatus('ready');
      setError(null);
    } catch (e) {
      if (e instanceof DaemonUnreachableError) {
        setStatus('offline');
        return;
      }
      setError(e instanceof Error ? e.message : String(e));
      setStatus('ready');
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const join = useCallback(
    async (platform: string, slug: string) => {
      await joinChannel(platform, slug);
      await refresh();
    },
    [refresh],
  );

  const leave = useCallback(
    async (platform: string, slug: string) => {
      await leaveChannel(platform, slug);
      await refresh();
    },
    [refresh],
  );

  return { channels, status, error, join, leave, refresh };
}
