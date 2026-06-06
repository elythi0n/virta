import { request } from './http';
import type { MintedToken, TokenInfo } from './wire.gen';

export function listTokens(): Promise<TokenInfo[]> {
  return request<{ tokens: TokenInfo[] }>('/v1/tokens').then((r) => r.tokens);
}

export function mintToken(name: string, scopes: string[]): Promise<MintedToken> {
  return request<MintedToken>('/v1/tokens', {
    method: 'POST',
    body: JSON.stringify({ name, scopes }),
  });
}

export function revokeToken(id: string): Promise<void> {
  return request<void>(`/v1/tokens/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
