import { request } from './http';
import { discover } from './discovery';
import type { AskEvent, IntelConfig, ModelGroup } from './wire.gen';

export function listModels(): Promise<ModelGroup[]> {
  return request<{ groups: ModelGroup[] }>('/v1/intel/models').then((r) => r.groups ?? []);
}

export function getIntelConfig(): Promise<IntelConfig> {
  return request<IntelConfig>('/v1/intel/config');
}

export function setIntelConfig(cfg: Partial<IntelConfig>): Promise<void> {
  return request<void>('/v1/intel/config', { method: 'PUT', body: JSON.stringify(cfg) });
}

// askStream opens a streaming /v1/intel/ask (NDJSON) request.
// Returns an abort function that cancels the stream.
export function askStream(
  question: string,
  model: string,
  onEvent: (ev: AskEvent) => void,
  onDone: () => void,
  onError: (msg: string) => void,
): () => void {
  const ctrl = new AbortController();
  (async () => {
    const d = await discover();
    if (!d) { onError('daemon not reachable'); return; }
    const base = d.addr ? `http://${d.addr}` : '';
    const res = await fetch(`${base}/v1/intel/ask`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${d.token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ question, model }),
      signal: ctrl.signal,
    }).catch((e: unknown) => { onError(e instanceof Error ? e.message : String(e)); return null; });
    if (!res) return;
    if (!res.ok) { onError(`${res.status} ${res.statusText}`); return; }
    const reader = res.body?.getReader();
    if (!reader) { onDone(); return; }
    const dec = new TextDecoder();
    let buf = '';
    while (true) {
      const { done, value } = await reader.read().catch(() => ({ done: true, value: undefined }));
      if (done) break;
      buf += dec.decode(value, { stream: true });
      const lines = buf.split('\n');
      buf = lines.pop() ?? '';
      for (const line of lines) {
        const t = line.trim();
        if (!t) continue;
        try { onEvent(JSON.parse(t) as AskEvent); } catch { /* ignore */ }
      }
    }
    onDone();
  })();
  return () => ctrl.abort();
}
