import type { FeedMessage } from '@virta/feed-core';
import type { Discovery, WireEvent } from './wire.gen';
import { toFeedMessage } from './normalize';

export type ConnectionStatus = 'offline' | 'connecting' | 'connected' | 'reconnecting';

export interface DaemonClient {
  start(): void;
  stop(): void;
}

export interface DaemonClientOptions {
  onMessage: (msg: FeedMessage) => void;
  onStatus: (status: ConnectionStatus) => void;
  /** Channel keys ("platform:slug") to receive; empty = all. */
  channels?: string[];
  /** Override discovery (tests / non-browser hosts). Default reads the shell's /__discovery. */
  discover?: () => Promise<Discovery | null>;
}

const BACKOFF_MS = [500, 1000, 2000, 5000];

// Connects to the daemon's /v1/stream WebSocket, applies live messages, and resumes after drops.
// The address and token come from discovery; the server replays buffered events past `since` on
// reconnect (at-least-once), so we dedupe by the monotonic seq.
export function createDaemonClient(opts: DaemonClientOptions): DaemonClient {
  const discover = opts.discover ?? defaultDiscover;
  let socket: WebSocket | null = null;
  let stopped = false;
  let lastSeq = 0;
  let attempt = 0;
  let retry: ReturnType<typeof setTimeout> | null = null;

  async function connect() {
    if (stopped) return;
    const d = await discover();
    if (!d || !d.addr) {
      opts.onStatus('offline'); // no daemon to reach (e.g. running the SPA without one)
      return;
    }
    opts.onStatus(attempt === 0 ? 'connecting' : 'reconnecting');
    const ws = new WebSocket(`ws://${d.addr}/v1/stream?token=${encodeURIComponent(d.token)}`);
    socket = ws;

    ws.onopen = () => {
      attempt = 0;
      opts.onStatus('connected');
      ws.send(JSON.stringify({ action: 'subscribe', channels: opts.channels ?? [], since: lastSeq }));
    };
    ws.onmessage = (ev) => {
      let event: WireEvent;
      try {
        event = JSON.parse(ev.data as string) as WireEvent;
      } catch {
        return;
      }
      if (typeof event.seq === 'number') {
        if (event.seq <= lastSeq) return; // already applied (replay overlap)
        lastSeq = event.seq;
      }
      if (event.type === 'message' && event.message) {
        opts.onMessage(toFeedMessage(event.message));
      }
    };
    ws.onerror = () => ws.close();
    ws.onclose = () => {
      socket = null;
      scheduleReconnect();
    };
  }

  function scheduleReconnect() {
    if (stopped) return;
    opts.onStatus('reconnecting');
    const delay = BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)];
    attempt += 1;
    retry = setTimeout(() => void connect(), delay);
  }

  return {
    start() {
      stopped = false;
      void connect();
    },
    stop() {
      stopped = true;
      if (retry) clearTimeout(retry);
      if (socket) {
        socket.onclose = null; // a deliberate stop should not trigger a reconnect
        socket.close();
        socket = null;
      }
    },
  };
}

async function defaultDiscover(): Promise<Discovery | null> {
  try {
    const res = await fetch('/__discovery');
    if (!res.ok) return null;
    const d = (await res.json()) as Discovery;
    return d.addr ? d : null;
  } catch {
    return null;
  }
}
