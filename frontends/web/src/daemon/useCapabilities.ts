import { useCallback, useEffect, useState } from 'react';
import { DaemonUnreachableError, getCapabilities } from './api';
import type { Capabilities } from './wire.gen';

export type CapabilitiesStatus = 'loading' | 'ready' | 'offline';

// Per-platform capabilities (read/send/moderation), which flip as accounts authenticate. Used to
// show whether a platform is signed in (send-capable).
export function useCapabilities() {
  const [caps, setCaps] = useState<Record<string, Capabilities>>({});
  const [status, setStatus] = useState<CapabilitiesStatus>('loading');

  const refresh = useCallback(async () => {
    try {
      setCaps(await getCapabilities());
      setStatus('ready');
    } catch (e) {
      if (e instanceof DaemonUnreachableError) {
        setStatus('offline');
        return;
      }
      setStatus('ready');
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { caps, status, refresh };
}
