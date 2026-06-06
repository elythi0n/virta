import { useCallback, useEffect, useMemo, useRef } from 'react';
import { Feed, useFeedBuffer } from '@virta/feed-core';
import { channelKey, toFeedMessage } from '../daemon';
import type { WireEvent } from '../daemon/wire.gen';
import styles from './Overlay.module.css';

// Read config from the overlay URL: ?channels=twitch:forsen,kick:xqc&max=80&token=...
function parseConfig() {
  const p = new URLSearchParams(location.search);
  const channels = p.get('channels')?.split(',').filter(Boolean) ?? [];
  const max = Math.min(500, Math.max(20, parseInt(p.get('max') ?? '80', 10)));
  const token = p.get('token') ?? '';
  return { channels, max, token };
}

// Transparent overlay rendering the live feed — designed for an OBS browser source.
// Every element uses semi-transparent backgrounds so the stream is visible through text.
// Mounting on transparent: no background-color at all on the root, only on text lines.
export default function Overlay() {
  const { channels, max, token } = useMemo(parseConfig, []);
  const { messages, push, markDeleted, clearChannel } = useFeedBuffer({ max });

  const lastSeq = useRef(0);

  const handleMessage = useCallback((ev: MessageEvent<string>) => {
    try {
      const e: WireEvent = JSON.parse(ev.data);
      if (typeof e.seq === 'number') {
        if (e.seq <= lastSeq.current) return;
        lastSeq.current = e.seq;
      }
      if (e.type === 'message' && e.message) {
        push(toFeedMessage(e.message));
      } else if (e.type === 'message_deleted') {
        markDeleted({ id: e.message_id || undefined, platformMessageId: e.platform_message_id || undefined });
      } else if (e.type === 'channel_clear' && e.channel) {
        const key = channelKey(e.channel.platform, e.channel.slug);
        clearChannel(key, e.target_user_id || undefined);
      }
    } catch {
      // ignore malformed frames
    }
  }, [push, markDeleted, clearChannel]);

  useEffect(() => {
    const base = location.origin.replace(/^http/, 'ws');
    const params = new URLSearchParams({ token });
    const ws = new WebSocket(`${base}/v1/stream?${params}`);
    ws.addEventListener('open', () => {
      ws.send(JSON.stringify({ action: 'subscribe', channels, since: 0 }));
    });
    ws.addEventListener('message', handleMessage);
    ws.addEventListener('close', () => {
      // Reconnect after a short delay; OBS sources can outlive daemon restarts.
      setTimeout(() => window.location.reload(), 3000);
    });
    return () => ws.close();
  }, [channels.join(','), token, handleMessage]);

  return (
    <div className={styles.root}>
      <Feed
        messages={messages}
        background="rgba(0,0,0,0)"
        showSource={channels.length !== 1}
        density="comfortable"
        showTimestamps
        celebrate={false}
      />
    </div>
  );
}
