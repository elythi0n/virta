import { request } from './http';
import type { AskEvent } from './wire.gen';

export interface ConversationSummary {
  id: string;
  title: string;
  model: string;
  updated_at: string;
}

export interface SaveConversationPayload {
  id: string;
  title: string;
  model: string;
  messages: unknown[];
}

export interface ConversationDetail {
  id: string;
  title: string;
  model: string;
  messages: unknown[]; // TurnItem[] serialised as JSON
  updated_at: string;
}

export function getConversation(id: string): Promise<ConversationDetail> {
  return request<ConversationDetail>(`/v1/intel/conversations/${encodeURIComponent(id)}`);
}

export function listConversations(): Promise<ConversationSummary[]> {
  return request<{ conversations: ConversationSummary[] }>('/v1/intel/conversations')
    .then(r => r.conversations);
}

export function saveConversation(payload: SaveConversationPayload): Promise<void> {
  return request('/v1/intel/conversations', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function deleteConversation(id: string): Promise<void> {
  return request(`/v1/intel/conversations/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

/** Stream a short AI-generated title. Same NDJSON format as askStream. */
export function generateTitleStream(
  message: string,
  model: string,
  onText: (chunk: string) => void,
  onDone: () => void,
): () => void {
  let aborted = false;
  (async () => {
    try {
      const base = (await import('./discovery').then(m => m.discover()))?.addr ?? '';
      const token = (await import('./discovery').then(m => m.discover()))?.token ?? '';
      const res = await fetch(`${base ? `http://${base}` : ''}/v1/intel/title`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ message, model }),
      });
      if (!res.ok || !res.body) { onDone(); return; }
      const reader = res.body.getReader();
      const dec = new TextDecoder();
      let buf = '';
      while (!aborted) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        const lines = buf.split('\n');
        buf = lines.pop() ?? '';
        for (const line of lines) {
          if (!line.trim()) continue;
          try {
            const ev: AskEvent = JSON.parse(line);
            if (ev.kind === 'text' && ev.text) onText(ev.text);
            if (ev.kind === 'done' || ev.kind === 'error') { onDone(); return; }
          } catch { /* skip malformed */ }
        }
      }
    } catch { /* ignore */ }
    onDone();
  })();
  return () => { aborted = true; };
}
