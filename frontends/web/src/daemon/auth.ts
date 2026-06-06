import { request } from './http';
import type { AuthSession, DeviceSession } from './wire.gen';

// Session states the daemon reports. `pending` is in-flight; the rest are terminal.
const TERMINAL = new Set(['authorized', 'denied', 'expired', 'error']);
export function isTerminal(state: string): boolean {
  return TERMINAL.has(state);
}

export const startTwitchDevice = (): Promise<DeviceSession> =>
  request<DeviceSession>('/v1/auth/twitch/device', { method: 'POST' });

export const twitchDeviceStatus = (id: string): Promise<DeviceSession> =>
  request<DeviceSession>(`/v1/auth/twitch/device/${encodeURIComponent(id)}`);

export const startKickAuth = (): Promise<AuthSession> =>
  request<AuthSession>('/v1/auth/kick/start', { method: 'POST' });

export const kickAuthStatus = (id: string): Promise<AuthSession> =>
  request<AuthSession>(`/v1/auth/kick/${encodeURIComponent(id)}`);

type PollOptions = {
  intervalMs: number;
  signal?: AbortSignal;
  /** Injectable for tests; defaults to a real timer. */
  sleep?: (ms: number) => Promise<void>;
};

// Poll a status function until the session reaches a terminal state or the signal aborts.
export async function pollSession<T extends { state: string }>(
  getStatus: () => Promise<T>,
  opts: PollOptions,
): Promise<T> {
  const sleep = opts.sleep ?? ((ms) => new Promise((r) => setTimeout(r, ms)));
  for (;;) {
    if (opts.signal?.aborted) throw new DOMException('aborted', 'AbortError');
    const session = await getStatus();
    if (isTerminal(session.state)) return session;
    await sleep(opts.intervalMs);
  }
}
