import { discover, resetDiscovery } from './discovery';

export class DaemonUnreachableError extends Error {
  constructor() {
    super('daemon not reachable');
    this.name = 'DaemonUnreachableError';
  }
}

// Authenticated REST call to the daemon, resolving its address + token from discovery. On a 401
// the cached discovery is dropped (token rotated) so the next call re-reads it.
export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const d = await discover();
  if (!d) throw new DaemonUnreachableError();

  const headers = new Headers(init?.headers);
  headers.set('Authorization', `Bearer ${d.token}`);
  if (init?.body) headers.set('Content-Type', 'application/json');

  const res = await fetch(`http://${d.addr}${path}`, { ...init, headers });
  if (res.status === 401) {
    resetDiscovery();
    throw new Error('unauthorized');
  }
  if (!res.ok) throw new Error(`${init?.method ?? 'GET'} ${path} -> ${res.status}`);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
