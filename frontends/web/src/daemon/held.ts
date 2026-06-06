import { useCallback, useEffect, useState } from 'react';
import { request } from './http';
import { useDaemonStream } from './useDaemonStream';
import type { HeldMessage } from './wire.gen';

// The AutoMod hold queue: messages a platform is holding for moderator review, plus approve
// (post) and deny (drop) by id.
export function listHeld(): Promise<HeldMessage[]> {
  return request<{ held: HeldMessage[] }>('/v1/held').then((r) => r.held);
}

export function approveHeld(id: string): Promise<void> {
  return request<void>(`/v1/held/${encodeURIComponent(id)}/approve`, { method: 'POST' });
}

export function denyHeld(id: string): Promise<void> {
  return request<void>(`/v1/held/${encodeURIComponent(id)}/deny`, { method: 'POST' });
}

export interface HeldQueue {
  held: HeldMessage[];
  approve: (id: string) => void;
  deny: (id: string) => void;
}

// Loads the current hold queue and keeps it live: a held event appends, a held_resolved event (or
// a successful local approve/deny) removes. Approve/deny remove optimistically and reconcile from
// the resolved event the daemon emits on success; a failure reloads the authoritative list.
export function useHeld(): HeldQueue {
  const [held, setHeld] = useState<HeldMessage[]>([]);

  const reload = useCallback(() => {
    listHeld()
      .then(setHeld)
      .catch(() => {
        /* daemon unreachable; keep the current view */
      });
  }, []);

  useEffect(() => reload(), [reload]);

  useDaemonStream({
    onMessage: () => {},
    onHeld: (h) => setHeld((cur) => (cur.some((x) => x.id === h.id) ? cur : [...cur, h])),
    onHeldResolved: (_channel, id) => setHeld((cur) => cur.filter((x) => x.id !== id)),
  });

  const resolve = useCallback(
    (id: string, run: (id: string) => Promise<void>) => {
      setHeld((cur) => cur.filter((x) => x.id !== id)); // optimistic
      run(id).catch(reload); // on failure, restore the authoritative list
    },
    [reload],
  );

  const approve = useCallback((id: string) => resolve(id, approveHeld), [resolve]);
  const deny = useCallback((id: string) => resolve(id, denyHeld), [resolve]);

  return { held, approve, deny };
}
