import { useCallback, useEffect, useState } from 'react';
import { Badge, Button, Input, Text } from '@virta/ui-kit';
import { request } from '../daemon/http';
import type { WebhookAttempt, WebhookEndpointInfo } from '../daemon/wire.gen';
import styles from './WebhooksPanel.module.css';

interface WebhooksResponse {
  endpoints: WebhookEndpointInfo[];
  event_catalog: string[];
}

function relTime(ms: number) {
  const d = Date.now() - ms;
  if (d < 60000) return 'just now';
  if (d < 3600000) return `${Math.floor(d / 60000)}m ago`;
  return `${Math.floor(d / 3600000)}h ago`;
}

export default function WebhooksPanel() {
  const [data, setData] = useState<WebhooksResponse>({ endpoints: [], event_catalog: [] });
  const [newName, setNewName] = useState('');
  const [newURL, setNewURL] = useState('');
  const [newSecret, setNewSecret] = useState('');
  const [newEvents, setNewEvents] = useState<string[]>([]);
  const [adding, setAdding] = useState(false);
  const [log, setLog] = useState<{ id: string; attempts: WebhookAttempt[] } | null>(null);
  const [error, setError] = useState('');

  const reload = useCallback(() => {
    request<WebhooksResponse>('/v1/webhooks').then(setData).catch(() => {});
  }, []);
  useEffect(() => reload(), [reload]);

  const add = async () => {
    if (!newName || !newURL || newEvents.length === 0) return;
    setError('');
    try {
      await request('/v1/webhooks', {
        method: 'POST',
        body: JSON.stringify({ name: newName, url: newURL, events: newEvents, secret: newSecret }),
      });
      setNewName(''); setNewURL(''); setNewSecret(''); setNewEvents([]);
      setAdding(false);
      reload();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const del = async (id: string) => {
    await request(`/v1/webhooks/${encodeURIComponent(id)}`, { method: 'DELETE' }).catch(() => {});
    reload();
  };

  const resume = async (id: string) => {
    await request(`/v1/webhooks/${encodeURIComponent(id)}/resume`, { method: 'POST' }).catch(() => {});
    reload();
  };

  const showLog = async (id: string) => {
    const r = await request<{ attempts: WebhookAttempt[] }>(`/v1/webhooks/${encodeURIComponent(id)}/log`).catch(() => null);
    if (r) setLog({ id, attempts: r.attempts ?? [] });
  };

  const toggleEvent = (ev: string) =>
    setNewEvents(s => s.includes(ev) ? s.filter(x => x !== ev) : [...s, ev]);

  return (
    <div className={styles.panel}>
      {data.endpoints.length === 0 && !adding ? (
        <div className={styles.empty}>
          <Text variant="ui" tone="subtle">No webhooks configured.</Text>
          <Text variant="meta" tone="subtle">Add an endpoint to receive events when raids, subs, or messages arrive.</Text>
          <Button variant="subtle" size="sm" onClick={() => setAdding(true)}>+ Add endpoint</Button>
        </div>
      ) : (
        <>
          <div className={styles.list}>
            {data.endpoints.map(ep => (
              <div key={ep.id} className={styles.endpoint}>
                <div className={styles.endMeta}>
                  <span className={styles.endName}>{ep.name}</span>
                  <span className={styles.endURL} title={ep.url}>{ep.url}</span>
                  <span className={styles.endEvents}>{ep.events.join(', ')}</span>
                </div>
                <div className={styles.endStatus}>
                  {ep.paused && <Badge tone="warn">Paused</Badge>}
                  {!ep.paused && ep.active && <Badge tone="ok">Active</Badge>}
                </div>
                <div className={styles.endActions}>
                  {ep.paused && <Button variant="ghost" size="sm" onClick={() => void resume(ep.id)}>Resume</Button>}
                  <Button variant="ghost" size="sm" onClick={() => void showLog(ep.id)}>Log</Button>
                  <Button variant="ghost" size="sm" onClick={() => void del(ep.id)}>Delete</Button>
                </div>
              </div>
            ))}
          </div>
          {!adding && <Button variant="subtle" size="sm" onClick={() => setAdding(true)}>+ Add endpoint</Button>}
        </>
      )}

      {adding && (
        <div className={styles.form}>
          <Text variant="meta" tone="subtle" as="h4" className={styles.formTitle}>New webhook endpoint</Text>
          <div className={styles.row2}>
            <div className={styles.fieldBlock}>
              <label className={styles.label}>Name</label>
              <Input aria-label="Endpoint name" placeholder="e.g. Raid lights" value={newName} onChange={e => setNewName(e.currentTarget.value)} />
            </div>
            <div className={styles.fieldBlock}>
              <label className={styles.label}>URL</label>
              <Input aria-label="Endpoint URL" placeholder="https://…" value={newURL} onChange={e => setNewURL(e.currentTarget.value)} />
            </div>
          </div>
          <div className={styles.fieldBlock}>
            <label className={styles.label}>Secret (optional — used for HMAC signature verification)</label>
            <Input aria-label="Webhook secret" type="password" placeholder="leave empty to skip HMAC" value={newSecret} onChange={e => setNewSecret(e.currentTarget.value)} />
          </div>
          <div className={styles.fieldBlock}>
            <label className={styles.label}>Events</label>
            <div className={styles.evGrid}>
              {data.event_catalog.map(ev => (
                <label key={ev} className={`${styles.evChip} ${newEvents.includes(ev) ? styles.evOn : ''}`}>
                  <input type="checkbox" className={styles.srOnly} checked={newEvents.includes(ev)} onChange={() => toggleEvent(ev)} />
                  {ev}
                </label>
              ))}
            </div>
          </div>
          {error && <Text variant="meta" tone="subtle" as="p" className={styles.error}>{error}</Text>}
          <div className={styles.formActions}>
            <Button variant="ghost" size="md" onClick={() => { setAdding(false); setError(''); }}>Cancel</Button>
            <Button variant="solid" size="md" disabled={!newName || !newURL || newEvents.length === 0} onClick={() => void add()}>Add</Button>
          </div>
        </div>
      )}

      {log && (
        <div className={styles.logModal}>
          <div className={styles.logHead}>
            <Text variant="ui" as="h4">Delivery log — {log.id}</Text>
            <button type="button" className={styles.closeBtn} aria-label="Close delivery log" onClick={() => setLog(null)}>×</button>
          </div>
          {log.attempts.length === 0 ? (
            <Text variant="meta" tone="subtle">No attempts recorded yet.</Text>
          ) : (
            <table className={styles.logTable}>
              <thead><tr><th>Time</th><th>Status</th><th>Latency</th><th>Error</th></tr></thead>
              <tbody>
                {[...log.attempts].reverse().map((a) => (
                  <tr key={`${a.at_ms}-${a.latency_ms}`} className={(a.status_code ?? 0) >= 200 && (a.status_code ?? 0) < 300 ? styles.rowOk : styles.rowFail}>
                    <td>{relTime(a.at_ms)}</td>
                    <td>{a.status_code || '—'}</td>
                    <td>{a.latency_ms}ms</td>
                    <td>{a.error || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}
