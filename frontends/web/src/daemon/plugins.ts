import { request } from './http';

export interface PluginInfo {
  id: string;
  name: string;
  version: string;
  publisher?: string;
  description?: string;
  tags?: string[];
  state: 'enabled' | 'disabled' | 'error' | 'installed';
  error?: string;
  built_in: boolean;
  scopes?: string[];
  has_config?: boolean;
}

export function listPlugins(): Promise<PluginInfo[]> {
  return request<{ plugins: PluginInfo[] }>('/v1/plugins').then(r => r.plugins);
}

export function enablePlugin(id: string): Promise<void> {
  return request(`/v1/plugins/${encodeURIComponent(id)}/enable`, { method: 'POST' });
}

export function disablePlugin(id: string): Promise<void> {
  return request(`/v1/plugins/${encodeURIComponent(id)}/disable`, { method: 'POST' });
}

export function installPlugin(url: string): Promise<PluginInfo> {
  return request<PluginInfo>('/v1/plugins/install', {
    method: 'POST',
    body: JSON.stringify({ url }),
  });
}

export function uninstallPlugin(id: string): Promise<void> {
  return request(`/v1/plugins/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export interface PluginDetail extends PluginInfo {
  config_schema?: Record<string, unknown>;
}

export function getPlugin(id: string): Promise<PluginDetail> {
  return request<PluginDetail>(`/v1/plugins/${encodeURIComponent(id)}`);
}
