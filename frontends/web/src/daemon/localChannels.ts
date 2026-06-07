import type { ChannelInfo } from './wire.gen';

const KEY = 'virta_guest_channels';

interface Stored {
  platform: string;
  slug: string;
}

function load(): Stored[] {
  try {
    const raw = localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as Stored[]) : [];
  } catch {
    return [];
  }
}

function save(channels: Stored[]): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(channels));
  } catch {
    // storage quota exceeded or private-browsing restriction — silently ignore
  }
}

export function addGuestChannel(platform: string, slug: string): void {
  const current = load();
  if (current.some(c => c.platform === platform && c.slug === slug)) return;
  save([...current, { platform, slug }]);
}

export function removeGuestChannel(platform: string, slug: string): void {
  save(load().filter(c => !(c.platform === platform && c.slug === slug)));
}

export function clearGuestChannels(): void {
  try { localStorage.removeItem(KEY); } catch { /* ignore */ }
}

/** Returns guest channels shaped as ChannelInfo with state 'local' (not connected). */
export function loadGuestChannelInfos(): ChannelInfo[] {
  return load().map(c => ({ platform: c.platform, slug: c.slug, state: 'local' }));
}

/** Returns the raw stored list for syncing to the server on login. */
export function loadGuestChannels(): Stored[] {
  return load();
}
