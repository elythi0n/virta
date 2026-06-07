import { useCallback, useEffect, useRef, useState } from 'react';
import { DaemonUnreachableError, joinChannel, leaveChannel, listChannels } from './api';
import { addGuestChannel, loadGuestChannelInfos, removeGuestChannel } from './localChannels';
import { useIsGuest } from './hostedAuth';
import type { ChannelInfo } from './wire.gen';

export type ChannelsStatus = 'loading' | 'ready' | 'offline';

export function useChannels() {
  const isGuest = useIsGuest();

  // ── Server state ────────────────────────────────────────────────────────────
  const [serverChannels, setServerChannels] = useState<ChannelInfo[]>([]);
  const [serverStatus, setServerStatus] = useState<ChannelsStatus>('loading');
  const [serverError, setServerError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setServerChannels(await listChannels());
      setServerStatus('ready');
      setServerError(null);
    } catch (e) {
      if (e instanceof DaemonUnreachableError) { setServerStatus('offline'); return; }
      setServerError(e instanceof Error ? e.message : String(e));
      setServerStatus('ready');
    }
  }, []);

  useEffect(() => {
    if (isGuest) return; // skip server polling — would return 401 anyway
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

  // ── Guest (localStorage) state ───────────────────────────────────────────────
  const [guestChannels, setGuestChannels] = useState<ChannelInfo[]>(() =>
    loadGuestChannelInfos(),
  );
  // Keep a ref so we can reload without stale closures.
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

  // Keep guest list fresh if another tab changes localStorage.
  const prevIsGuest = useRef(isGuest);
  useEffect(() => {
    // Reload when transitioning back to guest (e.g. after sign-out).
    if (isGuest && !prevIsGuest.current) reloadGuest();
    prevIsGuest.current = isGuest;
  }, [isGuest, reloadGuest]);

  // ── Return appropriate slice ─────────────────────────────────────────────────
  if (isGuest) {
    return {
      channels: guestChannels,
      status: 'ready' as ChannelsStatus,
      error: null,
      join: guestJoin,
      leave: guestLeave,
      refresh: reloadGuest,
    };
  }

  return {
    channels: serverChannels,
    status: serverStatus,
    error: serverError,
    join: serverJoin,
    leave: serverLeave,
    refresh,
  };
}
