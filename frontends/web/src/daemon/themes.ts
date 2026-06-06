import { request } from './http';
import type { ThemeInfo } from './wire.gen';

export function listThemes(): Promise<ThemeInfo[]> {
  return request<{ themes: ThemeInfo[] }>('/v1/themes').then((r) => r.themes ?? []);
}

export function importTheme(data: object): Promise<ThemeInfo> {
  return request<ThemeInfo>('/v1/themes', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export function exportTheme(id: string): Promise<Blob> {
  return fetch(`/v1/themes/${encodeURIComponent(id)}/export`).then((r) => r.blob());
}

export function deleteTheme(id: string): Promise<void> {
  return request<void>(`/v1/themes/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
