import { request } from './http';
import type { AccountInfo } from './wire.gen';

// Connected accounts (identity + scopes; never the token).
export function listAccounts(): Promise<AccountInfo[]> {
  return request<{ accounts: AccountInfo[] }>('/v1/accounts').then((r) => r.accounts);
}

// Disconnect an account: the daemon removes its secret + row and reverts the platform to read-only.
export function disconnectAccount(id: string): Promise<void> {
  return request<void>(`/v1/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
