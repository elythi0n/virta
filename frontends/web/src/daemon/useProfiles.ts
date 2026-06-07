import { useCallback, useEffect, useState } from 'react';
import { DaemonUnreachableError } from './api';
import { activateProfile, createProfile, deleteProfile, listProfiles } from './profiles';
import type { ProfileInfo } from './wire.gen';

export type ProfilesStatus = 'loading' | 'ready' | 'offline';

// Loads workspace profiles and exposes activate/create that refresh the list.
export function useProfiles() {
  const [profiles, setProfiles] = useState<ProfileInfo[]>([]);
  const [status, setStatus] = useState<ProfilesStatus>('loading');

  const refresh = useCallback(async () => {
    try {
      setProfiles(await listProfiles());
      setStatus('ready');
    } catch (e) {
      setStatus(e instanceof DaemonUnreachableError ? 'offline' : 'ready');
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const activate = useCallback(
    async (id: string) => {
      await activateProfile(id);
      await refresh();
    },
    [refresh],
  );

  const create = useCallback(
    async (name: string) => {
      await createProfile(name);
      await refresh();
    },
    [refresh],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteProfile(id);
      await refresh();
    },
    [refresh],
  );

  const active = profiles.find((p) => p.active);
  return { profiles, active, status, activate, create, remove, refresh };
}
