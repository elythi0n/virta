import { request } from './http';
import type { Capabilities, ChannelInfo } from './wire.gen';

export { DaemonUnreachableError } from './http';

export function listChannels(): Promise<ChannelInfo[]> {
  return request<{ channels: ChannelInfo[] }>('/v1/channels').then((r) => r.channels);
}

export function joinChannel(platform: string, slug: string, mode?: string): Promise<void> {
  return request('/v1/channels', { method: 'POST', body: JSON.stringify({ platform, slug, mode }) });
}

export function leaveChannel(platform: string, slug: string): Promise<void> {
  const q = `platform=${encodeURIComponent(platform)}&slug=${encodeURIComponent(slug)}`;
  return request(`/v1/channels?${q}`, { method: 'DELETE' });
}

export function getCapabilities(): Promise<Record<string, Capabilities>> {
  return request<{ capabilities: Record<string, Capabilities> }>('/v1/capabilities').then((r) => r.capabilities);
}
