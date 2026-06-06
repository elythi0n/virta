import { useEffect, useState } from 'react';
import { request } from './http';
import { channelKey } from './normalize';
import type { StreamInfo } from './wire.gen';

// Live stream metadata (viewer count, title, category, thumbnail) per joined channel.
export function listStreams(): Promise<StreamInfo[]> {
  return request<{ streams: StreamInfo[] }>('/v1/streams').then((r) => r.streams);
}

// Polls /v1/streams while mounted (the daemon refreshes the underlying snapshots lazily), keyed by
// "platform:slug". Stream metadata changes slowly, so a relaxed cadence is plenty.
export function useStreams(): Record<string, StreamInfo> {
  const [streams, setStreams] = useState<Record<string, StreamInfo>>({});
  useEffect(() => {
    let cancelled = false;
    const poll = async () => {
      try {
        const list = await listStreams();
        if (cancelled) return;
        const next: Record<string, StreamInfo> = {};
        for (const s of list) next[channelKey(s.platform, s.slug)] = s;
        setStreams(next);
      } catch {
        // daemon unreachable; keep the previous snapshot rather than blanking the rail
      }
    };
    void poll();
    const id = setInterval(poll, 20000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);
  return streams;
}
