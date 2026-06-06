import { request } from './http';
import type { AuthConfig } from './wire.gen';

// OAuth app credentials (which platforms have a client id; the id for editing, never the secret).
export function getAuthConfig(): Promise<AuthConfig> {
  return request<AuthConfig>('/v1/auth/config');
}

// Set a platform's OAuth app client id (+ optional secret); persisted to the daemon's vault.
export function setAuthConfig(platform: string, clientId: string, clientSecret = ''): Promise<AuthConfig> {
  return request<AuthConfig>('/v1/auth/config', {
    method: 'PUT',
    body: JSON.stringify({ platform, client_id: clientId, client_secret: clientSecret }),
  });
}
