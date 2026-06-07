import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { DaemonUnreachableError, joinChannel, leaveChannel, listChannels } from './api';
import { addGuestChannel, loadGuestChannelInfos, removeGuestChannel } from './localChannels';
import { useIsGuest } from './hostedAuth';
import type { ChannelInfo } from './wire.gen';

export type ChannelsStatus = 'loading' | 'ready' | 'offline';

interface ChannelsValue {
  channels: ChannelInfo[];
  status: ChannelsStatus;
  error: string | null;
  join: (platform: string, slug: string) => Promise<void>;
  leave: (platform: string, slug: string) => Promise<void>;
  refresh: () => void | Promise<void>;
}

const ChannelsContext = createContext<ChannelsValue | null>(null);

export function ChannelsProvider({ children }: { children: ReactNode }) {
  const isGuest = useIsGuest();

  // Server state
  const [serverChannels, setServerChannels] = useState<ChannelInfo[]>([]);
  const [serverStatus, setServerStatus] = useState<ChannelsStatus>('loading');
  const [serverError, setServerError] = useState<string | null>(null);

  // Stable-reference guard: only update state when the list actually changed.
  const listRef = useRef<ChannelInfo[]>([]);

  const refresh = useCallback(async () => {
    try {
      const fetched = await listChannels();
      const sig = JSON.stringify(fetched);
      if (sig !== JSON.stringify(listRef.current)) {
        listRef.current = fetched;
        setServerChannels(fetched);
      }
      setServerStatus('ready');
      setServerError(null);
    } catch (e) {
      if (e instanceof DaemonUnreachableError) { setServerStatus('offline'); return; }
      setServerError(e instanceof Error ? e.message : String(e));
      setServerStatus('ready');
    }
  }, []);

  useEffect(() => {
    if (isGuest) return;
    void refresh();
    const id = setInterval(() => void refresh(), 5000);
    return () => clearInterval(id);
  }, [refresh, isGuest]);

  const serverJoin = useCallback(async (platform: string, slug: string) => {
    await joinChannel(platform, slug);
    await refresh();
  }, [refresh]);

  const serverLeave = useCallback(async (platform: string, slug: string) => {
    await leaveChannel(platform, slug);
    await refresh();
  }, [refresh]);

  // Guest (localStorage) state
  const [guestChannels, setGuestChannels] = useState<ChannelInfo[]>(() =>
    loadGuestChannelInfos(),
  );
  const reloadGuest = useCallback(() => setGuestChannels(loadGuestChannelInfos()), []);

  const guestJoin = useCallback((_platform: string, _slug: string) => {
    addGuestChannel(_platform, _slug);
    reloadGuest();
    return Promise.resolve();
  }, [reloadGuest]);

  const guestLeave = useCallback((_platform: string, _slug: string) => {
    removeGuestChannel(_platform, _slug);
    reloadGuest();
    return Promise.resolve();
  }, [reloadGuest]);

  const prevIsGuest = useRef(isGuest);
  useEffect(() => {
    if (isGuest && !prevIsGuest.current) reloadGuest();
    prevIsGuest.current = isGuest;
  }, [isGuest, reloadGuest]);

  const value: ChannelsValue = isGuest
    ? {
        channels: guestChannels,
        status: 'ready' as ChannelsStatus,
        error: null,
        join: guestJoin,
        leave: guestLeave,
        refresh: reloadGuest,
      }
    : {
        channels: serverChannels,
        status: serverStatus,
        error: serverError,
        join: serverJoin,
        leave: serverLeave,
        refresh,
      };

  return (
    <ChannelsContext.Provider value={value}>
      {children}
    </ChannelsContext.Provider>
  );
}

export function useChannels(): ChannelsValue {
  const ctx = useContext(ChannelsContext);
  if (!ctx) throw new Error('useChannels must be used inside ChannelsProvider');
  return ctx;
}
