import type { Discovery } from './wire.gen';

// The shell serves the daemon's address + token at /__discovery (same origin). Both the WS client
// and the REST client read it from here. Cached for the session; reset on an auth failure.
let cached: Promise<Discovery | null> | null = null;

export function discover(): Promise<Discovery | null> {
  if (!cached) cached = fetchDiscovery();
  return cached;
}

export function resetDiscovery(): void {
  cached = null;
}

async function fetchDiscovery(): Promise<Discovery | null> {
  try {
    const res = await fetch('/__discovery');
    if (!res.ok) return null;
    const d = (await res.json()) as Discovery;
    return d.addr ? d : null;
  } catch {
    return null;
  }
}
