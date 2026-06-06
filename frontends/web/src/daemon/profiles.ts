import { request } from './http';
import type { ProfileInfo } from './wire.gen';

export function listProfiles(): Promise<ProfileInfo[]> {
  return request<{ profiles: ProfileInfo[] }>('/v1/profiles').then((r) => r.profiles);
}

export function createProfile(name: string): Promise<ProfileInfo> {
  return request<ProfileInfo>('/v1/profiles', { method: 'POST', body: JSON.stringify({ name }) });
}

export function activateProfile(id: string): Promise<void> {
  return request(`/v1/profiles/${encodeURIComponent(id)}/activate`, { method: 'POST' });
}
