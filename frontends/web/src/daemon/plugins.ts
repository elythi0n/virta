import { request } from './http';

export interface PluginPanelContrib {
  kind: string;
  title: string;
  icon?: string;
}

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
  panels?: PluginPanelContrib[];
  has_gui?: boolean;
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
  config?: Record<string, unknown>;
}

export function getPlugin(id: string): Promise<PluginDetail> {
  return request<PluginDetail>(`/v1/plugins/${encodeURIComponent(id)}`);
}

export function getPluginConfig(id: string): Promise<Record<string, unknown>> {
  return request<{ config: Record<string, unknown> }>(`/v1/plugins/${encodeURIComponent(id)}/config`).then(r => r.config ?? {});
}

export function putPluginConfig(id: string, values: Record<string, unknown>): Promise<void> {
  return request(`/v1/plugins/${encodeURIComponent(id)}/config`, {
    method: 'PUT',
    body: JSON.stringify(values),
  });
}

export interface PluginHttpRequest {
  url: string;
  method?: 'GET' | 'POST';
  headers?: Record<string, string>;
  body?: string;
}

export interface PluginHttpResponse {
  status: number;
  content_type?: string;
  body: string;
}

/** Bridged HTTP request on a plugin's behalf (the daemon enforces the declared-endpoint allowlist). */
export function pluginHttp(id: string, req: PluginHttpRequest): Promise<PluginHttpResponse> {
  return request<PluginHttpResponse>(`/v1/plugins/${encodeURIComponent(id)}/http`, {
    method: 'POST',
    body: JSON.stringify(req),
  });
}
