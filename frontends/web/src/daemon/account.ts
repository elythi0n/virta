import { request } from './http';

export interface VirtaUser {
  id: string;
  email: string;
  display_name: string;
}

export interface HostedStatus {
  hosted: boolean;
}

export async function getHostedStatus(): Promise<HostedStatus> {
  return request<HostedStatus>('/auth/status');
}

export async function register(email: string, displayName: string, password: string): Promise<{ user: VirtaUser }> {
  return request('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ email, display_name: displayName, password }),
  });
}

export async function login(email: string, password: string): Promise<{ user: VirtaUser }> {
  return request('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
}

export async function logout(): Promise<void> {
  return request('/auth/logout', { method: 'POST' });
}

export async function getMe(): Promise<VirtaUser> {
  return request<VirtaUser>('/auth/me');
}
