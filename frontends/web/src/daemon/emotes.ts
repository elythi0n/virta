import { useEffect, useState } from 'react';
import { request } from './http';
import type { EmoteInfo } from './wire.gen';

// The usable emotes across joined channels (code + ready-to-show url), for composer autocomplete.
export function listEmotes(): Promise<EmoteInfo[]> {
  return request<{ emotes: EmoteInfo[] }>('/v1/emotes').then((r) => r.emotes);
}

// Loads the emote set and refreshes slowly (the set changes only when channels join/refresh).
export function useEmotes(): EmoteInfo[] {
  const [emotes, setEmotes] = useState<EmoteInfo[]>([]);
  useEffect(() => {
    let cancelled = false;
    const poll = () =>
      listEmotes()
        .then((e) => !cancelled && setEmotes(e))
        .catch(() => {});
    void poll();
    const id = setInterval(poll, 60000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);
  return emotes;
}
