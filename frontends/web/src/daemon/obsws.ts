import { request } from './http';

export interface OBSStatus {
  state: 'disconnected' | 'connecting' | 'connected' | 'error';
  obs_version?: string;
  websocket_version?: string;
  error?: string;
}

export interface DataMapping {
  stat: 'msgs_per_min' | 'unique_chatters';
  source_name: string;
}

export interface EventRule {
  trigger: 'raid' | 'sub' | 'sub_gift' | 'resub';
  channel_key: string;
  action: 'switch_scene';
  target: string;
}

export interface OBSConfig {
  enabled: boolean;
  host: string;
  port: number;
  has_password: boolean;
  data_mappings: DataMapping[];
  event_rules: EventRule[];
  update_interval_s: number;
}

export interface OBSSourceInfo {
  name: string;
  kind: string;
}

export interface OBSSceneList {
  scenes: string[];
  current: string;
}

export function getOBSStatus(): Promise<OBSStatus> {
  return request<OBSStatus>('/v1/obsws/status');
}

export function getOBSConfig(): Promise<{ config: OBSConfig; has_password: boolean }> {
  return request<{ config: OBSConfig; has_password: boolean }>('/v1/obsws/config');
}

export function setOBSConfig(cfg: OBSConfig, password?: string): Promise<void> {
  const body: Record<string, unknown> = { config: cfg };
  if (password !== undefined) body.password = password;
  return request('/v1/obsws/config', { method: 'PUT', body: JSON.stringify(body) });
}

export function getOBSSources(): Promise<{ sources: OBSSourceInfo[] }> {
  return request<{ sources: OBSSourceInfo[] }>('/v1/obsws/sources');
}

export function getOBSScenes(): Promise<OBSSceneList> {
  return request<OBSSceneList>('/v1/obsws/scenes');
}

export function testOBSSource(sourceName: string, value: string): Promise<void> {
  return request('/v1/obsws/test-source', { method: 'POST', body: JSON.stringify({ source_name: sourceName, value }) });
}

export function detectOBS(): Promise<{ detected: boolean }> {
  return request<{ detected: boolean }>('/v1/obsws/detect', { method: 'POST', body: JSON.stringify({}) });
}
