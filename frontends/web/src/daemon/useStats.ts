import { useEffect, useRef, useState } from 'react';
import type { ConnectionStatus } from './client';
import type { StatsSnapshot } from './wire.gen';
import { useSharedStream } from './sharedStream';

export interface ChannelStats {
  messagesPerSec: number;
  uniqueChatters: number;
  windowSeconds: number;
  topEmote?: string;
}

function toChannelStats(s: StatsSnapshot): ChannelStats {
  return {
    messagesPerSec: s.messages_per_sec,
    uniqueChatters: s.unique_chatters,
    windowSeconds: s.window_seconds,
    topEmote: s.top_emotes?.[0]?.name,
  };
}

// Subscribes to the daemon for live per-channel stats (msg/s, unique chatters, top emote), keyed by
// "platform:slug". Mounted only while the streams view is open, so the firehose subscription is
// scoped to when it's actually needed. Stats arrive on a ~1/s ticker per channel.
export function useStats(): { stats: Record<string, ChannelStats>; status: ConnectionStatus } {
  const conn = useSharedStream();
  const [stats, setStats] = useState<Record<string, ChannelStats>>({});
  const [status, setStatus] = useState<ConnectionStatus>(() => conn.getStatus());
  // Stable per-mount subscriber ID.
  const idRef = useRef<string | null>(null);
  if (!idRef.current) {
    idRef.current = Math.random().toString(36).slice(2);
  }
  const stableId = idRef.current;

  useEffect(() => {
    const off = conn.onStatusChange(setStatus);
    setStatus(conn.getStatus());
    conn.subscribe(stableId, {
      onMessage: () => {}, // stats view only needs stats, not the message stream
      onStats: (key, snapshot) => setStats((prev) => ({ ...prev, [key]: toChannelStats(snapshot) })),
    });
    return () => {
      conn.unsubscribe(stableId);
      off();
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { stats, status };
}
