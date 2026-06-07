import type { Discovery } from './wire.gen';

// The discovery bootstrap is either injected inline by the daemon when it serves index.html
// (window.__VIRTA_DISCOVERY__), or fetched from /__discovery as a fallback for local dev
// (Vite dev server) or same-machine loopback access.
//
// Injection is the primary path in Docker / production: the daemon writes
//   <script>window.__VIRTA_DISCOVERY__ = {addr:"",token:"<TOKEN>"};</script>
// into index.html at serve time, so the SPA authenticates immediately without a
// second network round-trip and without depending on IP routing being loopback.
declare global {
  interface Window {
    __VIRTA_DISCOVERY__?: { addr: string; token: string };
  }
}

let cached: Promise<Discovery | null> | null = null;

export function discover(): Promise<Discovery | null> {
  if (!cached) cached = resolveDiscovery();
  return cached;
}

export function resetDiscovery(): void {
  cached = null;
}

async function resolveDiscovery(): Promise<Discovery | null> {
  // 1. Injected bootstrap — fastest path, works in Docker and any reverse-proxy setup.
  //    Guard against SSR / Node.js test environments where window is undefined.
  if (typeof window !== 'undefined' && window.__VIRTA_DISCOVERY__?.token) {
    return { addr: window.__VIRTA_DISCOVERY__.addr ?? '', token: window.__VIRTA_DISCOVERY__.token };
  }

  // 2. Fetch from /__discovery — works when running with a local Vite dev server (which
  //    proxies /v1 to the daemon) or in same-machine loopback access.
  try {
    const res = await fetch('/__discovery');
    if (!res.ok) return null;
    const d = (await res.json()) as Discovery;
    return d.addr || d.token ? d : null;
  } catch {
    return null;
  }
}
