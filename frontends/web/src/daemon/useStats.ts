import { useEffect, useState } from 'react';
import { createDaemonClient, type ConnectionStatus } from './client';
import type { StatsSnapshot } from './wire.gen';

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
  const [stats, setStats] = useState<Record<string, ChannelStats>>({});
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  useEffect(() => {
    const client = createDaemonClient({
      onMessage: () => {}, // the streams view only needs stats, not the message stream
      onStats: (key, snapshot) => setStats((prev) => ({ ...prev, [key]: toChannelStats(snapshot) })),
      onStatus: setStatus,
    });
    client.start();
    return () => client.stop();
  }, []);
  return { stats, status };
}
