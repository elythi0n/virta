import { createContext, useContext, useEffect, useRef, type ReactNode } from 'react';
import type { DeletionRef, FeedMessage } from '@virta/feed-core';
import { createDaemonClient, type ConnectionStatus, type DaemonClient } from './client';
import type { ChatSettings, HeldMessage, Moment, StatsSnapshot } from './wire.gen';

// The full set of event callbacks a subscriber can register.
export interface SharedStreamHandlers {
  onMessage: (msg: FeedMessage) => void;
  onDeleted?: (ref: DeletionRef) => void;
  onClear?: (channelKey: string, userId?: string) => void;
  onChatSettings?: (channelKey: string, settings: ChatSettings) => void;
  onHeld?: (held: HeldMessage) => void;
  onHeldResolved?: (channelKey: string, id: string, approved: boolean) => void;
  onPlugin?: (stream: string, data: unknown) => void;
  onStats?: (channelKey: string, snapshot: StatsSnapshot) => void;
  onMoment?: (channelKey: string, moment: Moment) => void;
}

interface Subscription {
  handlers: SharedStreamHandlers;
  // undefined means "all channels"; an empty array also means all channels.
  channels?: string[];
}

type StatusListener = (status: ConnectionStatus) => void;

// Holds one DaemonClient and fans events out to all registered subscribers.
export class SharedConnection {
  private client: DaemonClient;
  private subs: Map<string, Subscription> = new Map();
  private statusListeners: Set<StatusListener> = new Set();
  private currentStatus: ConnectionStatus = 'connecting';

  constructor() {
    this.client = createDaemonClient({
      onMessage: (msg) => this.dispatchMessage(msg),
      onDeleted: (ref) => this.subs.forEach((s) => s.handlers.onDeleted?.(ref)),
      onClear: (key, userId) => this.subs.forEach((s) => s.handlers.onClear?.(key, userId)),
      onChatSettings: (key, settings) => this.subs.forEach((s) => s.handlers.onChatSettings?.(key, settings)),
      onHeld: (held) => this.subs.forEach((s) => s.handlers.onHeld?.(held)),
      onHeldResolved: (key, id, approved) => this.subs.forEach((s) => s.handlers.onHeldResolved?.(key, id, approved)),
      onPlugin: (stream, data) => this.subs.forEach((s) => s.handlers.onPlugin?.(stream, data)),
      onStats: (key, snapshot) => this.subs.forEach((s) => s.handlers.onStats?.(key, snapshot)),
      onMoment: (key, moment) => this.dispatchMoment(key, moment),
      onStatus: (status) => {
        this.currentStatus = status;
        this.statusListeners.forEach((fn) => fn(status));
      },
      // Start with an empty (all-channels) subscription; updateChannels will narrow it once
      // subscribers register with a specific channel set.
      channels: [],
    });
  }

  private dispatchMessage(msg: FeedMessage) {
    for (const sub of this.subs.values()) {
      // A subscriber with no channel filter (or an empty filter) receives every message.
      if (!sub.channels || sub.channels.length === 0) {
        sub.handlers.onMessage(msg);
        continue;
      }
      // Route only to the subs whose filter includes this message's channel.
      const msgKey = msg.channel;
      if (msgKey && sub.channels.includes(msgKey)) {
        sub.handlers.onMessage(msg);
      }
    }
  }

  // Moments route by the moment's channel key, same as messages: an unfiltered subscriber gets
  // them all; a channel-scoped subscriber only gets its own channels' moments.
  private dispatchMoment(key: string, moment: Moment) {
    for (const sub of this.subs.values()) {
      if (!sub.channels || sub.channels.length === 0 || sub.channels.includes(key)) {
        sub.handlers.onMoment?.(key, moment);
      }
    }
  }

  // Recompute the union of all requested channels and push it to the socket.
  private syncChannels() {
    const allKeys = new Set<string>();
    let wantAll = false;
    for (const sub of this.subs.values()) {
      if (!sub.channels || sub.channels.length === 0) {
        wantAll = true;
        break;
      }
      sub.channels.forEach((c) => allKeys.add(c));
    }
    // Empty array sent to the server means "all channels".
    this.client.updateChannels(wantAll ? [] : [...allKeys]);
  }

  subscribe(id: string, handlers: SharedStreamHandlers, channels?: string[]) {
    this.subs.set(id, { handlers, channels });
    this.syncChannels();
  }

  unsubscribe(id: string) {
    this.subs.delete(id);
    this.syncChannels();
  }

  onStatusChange(fn: StatusListener): () => void {
    this.statusListeners.add(fn);
    return () => this.statusListeners.delete(fn);
  }

  getStatus(): ConnectionStatus {
    return this.currentStatus;
  }

  start() {
    this.client.start();
  }

  stop() {
    this.client.stop();
  }
}

// Module-level singleton so the same instance survives HMR re-imports.
const singleton = new SharedConnection();

const SharedDaemonStreamContext = createContext<SharedConnection>(singleton);

export function SharedDaemonStreamProvider({ children }: { children: ReactNode }) {
  const connRef = useRef<SharedConnection>(singleton);
  useEffect(() => {
    connRef.current.start();
    return () => connRef.current.stop();
  }, []);
  return (
    <SharedDaemonStreamContext.Provider value={connRef.current}>
      {children}
    </SharedDaemonStreamContext.Provider>
  );
}

export function useSharedStream(): SharedConnection {
  return useContext(SharedDaemonStreamContext);
}
