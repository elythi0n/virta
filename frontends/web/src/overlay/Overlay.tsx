import { useCallback, useEffect, useMemo, useRef } from 'react';
import { Feed, useFeedBuffer } from '@virta/feed-core';
import { channelKey, toFeedMessage } from '../daemon';
import type { WireEvent } from '../daemon/wire.gen';
import { parseOverlayConfig } from './overlayConfig';
import styles from './Overlay.module.css';

export default function Overlay() {
  const cfg = useMemo(parseOverlayConfig, []);
  const { messages, push, markDeleted, clearChannel } = useFeedBuffer({ max: cfg.maxMessages });
  const lastSeq = useRef(0);
  const fadeTimers = useRef(new Map<string, ReturnType<typeof setTimeout>>());

  const scheduleFade = useCallback((id: string) => {
    if (!cfg.fadeMs) return;
    const prev = fadeTimers.current.get(id);
    if (prev) clearTimeout(prev);
    const t = setTimeout(() => {
      markDeleted({ id });
      fadeTimers.current.delete(id);
    }, cfg.fadeMs);
    fadeTimers.current.set(id, t);
  }, [cfg.fadeMs, markDeleted]);

  const handleMessage = useCallback((ev: MessageEvent<string>) => {
    try {
      const e: WireEvent = JSON.parse(ev.data);
      if (typeof e.seq === 'number') {
        if (e.seq <= lastSeq.current) return;
        lastSeq.current = e.seq;
      }
      if (e.type === 'message' && e.message) {
        const m = e.message;
        const isChat = !m.type || m.type === 'chat' || m.type === 'action';
        const isEvent = !isChat;
        // Kind-based filtering: drop messages that don't match the overlay's purpose.
        if (cfg.kind === 'chat' && isEvent) return;
        if (cfg.kind === 'events' && isChat) return;
        // 'mentions' kind: only include messages where the body contains at least one
        // of the channel's mention names. Since the overlay has no access to user names,
        // all chat is passed through — the user can use the FeedPanel's mention filter
        // in the main app for name-specific filtering. Mentions overlay = events + chat.
        const fm = toFeedMessage(m);
        push(fm);
        scheduleFade(fm.id);
      } else if (e.type === 'message_deleted') {
        markDeleted({ id: e.message_id || undefined, platformMessageId: e.platform_message_id || undefined });
      } else if (e.type === 'channel_clear' && e.channel) {
        clearChannel(channelKey(e.channel.platform, e.channel.slug), e.target_user_id || undefined);
      }
    } catch { /* ignore malformed frames */ }
  }, [cfg.kind, push, markDeleted, clearChannel, scheduleFade]);

  useEffect(() => {
    const wsBase = location.origin.replace(/^http/, 'ws');
    let dead = false;
    let retryDelay = 1000;
    let ws: WebSocket;
    const connect = () => {
      if (dead) return;
      ws = new WebSocket(`${wsBase}/v1/stream?token=${encodeURIComponent(cfg.token)}`);
      ws.addEventListener('open', () => {
        retryDelay = 1000;
        ws.send(JSON.stringify({ action: 'subscribe', channels: cfg.channels, since: lastSeq.current }));
      });
      ws.addEventListener('message', handleMessage);
      ws.addEventListener('close', () => {
        if (!dead) setTimeout(connect, retryDelay = Math.min(retryDelay * 1.5, 15000));
      });
    };
    connect();
    return () => { dead = true; ws?.close(); };
  // deps: channel list and token are stable strings; handleMessage is memoized
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cfg.channels.join(','), cfg.token, handleMessage]);

  const rootStyle: Record<string, string> = {};
  if (cfg.fontSize) rootStyle['--overlay-font-size'] = `${cfg.fontSize}px`;
  if (cfg.width) rootStyle['--overlay-width'] = `${cfg.width}px`;

  return (
    <div
      className={[
        styles.root,
        styles[`theme-${cfg.theme}`],
        styles[`align-${cfg.align}`],
        cfg.textShadow ? styles.textShadow : '',
      ].filter(Boolean).join(' ')}
      style={rootStyle as React.CSSProperties}
    >
      <Feed
        messages={messages}
        background="rgba(0,0,0,0)"
        showSource={cfg.showSource}
        showTimestamps={cfg.showTimestamps}
        density={cfg.density}
        celebrate={cfg.kind === 'celebrations'}
      />
    </div>
  );
}
