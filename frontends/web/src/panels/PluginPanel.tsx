import { useCallback, useEffect, useRef, useState } from 'react';
import { setPluginPatterns, removePluginPatterns } from '../decorators';
import { discover } from '../daemon/discovery';
import {
  getPlugin,
  getPluginConfig,
  putPluginConfig,
  pluginHttp,
  type PluginDetail,
  type PluginHttpRequest,
} from '../daemon/plugins';
import { mintToken } from '../daemon/tokens';
import { useSharedStream } from '../daemon/sharedStream';
import styles from './Panel.module.css';

// Message shape posted by the plugin's __virta.js SDK bootstrap.
interface BridgeMessage {
  __virta: true;
  seq: number;
  plugin: string;
  msg: { type: string; payload?: unknown };
}

/**
 * Generic host for a remote plugin's sandboxed GUI. The plugin's static assets are served by the
 * daemon under /plugins/{id}/gui/ with a strict CSP (no outbound network); everything privileged
 * flows through the postMessage bridge here, which calls the daemon's authenticated API:
 *
 *   config.get             → GET  /v1/plugins/{id}/config
 *   config.set {values}    → PUT  /v1/plugins/{id}/config        ('storage' scope)
 *   http.fetch {url,...}   → POST /v1/plugins/{id}/http (manifest-declared endpoints only)
 *   events.subscribe       → forwards live FeedMessages into the iframe as 'events.message'
 *                            posts ('events' scope; optional {channels} narrows the set)
 *   events.unsubscribe     → stops forwarding
 *   overlay.url {path}     → tokenized standalone URL for the plugin GUI file (OBS browser
 *                            sources). Mints a read-scoped API token once and persists it in the
 *                            plugin's config as overlay_token, so the URL stays stable.
 *
 * Scope checks here gate the bridge surface to what the manifest declared; the daemon separately
 * enforces config/http per plugin. Origins are pinned on both sides: the iframe only talks to our
 * origin (passed as ?host=), and we only accept messages from the daemon origin serving the iframe.
 */
export default function PluginPanel({ pluginId }: { pluginId: string }) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [src, setSrc] = useState<string | null>(null);
  const [frameOrigin, setFrameOrigin] = useState<string | null>(null);
  const [unreachable, setUnreachable] = useState(false);
  const conn = useSharedStream();
  // Manifest detail (for scope checks), fetched once per panel instance.
  const detailRef = useRef<Promise<PluginDetail> | null>(null);
  const subscribedRef = useRef(false);
  // Stable per-mount stream-subscriber ID so each plugin panel owns its own slot.
  const subIdRef = useRef<string | null>(null);
  if (!subIdRef.current) {
    subIdRef.current = `plugin:${pluginId}:${Math.random().toString(36).slice(2)}`;
  }

  useEffect(() => {
    let alive = true;
    discover().then((d) => {
      if (!alive) return;
      if (!d) {
        setUnreachable(true);
        return;
      }
      // Empty addr = the daemon serves this origin; otherwise the daemon is on its own origin.
      const base = d.addr ? `http://${d.addr}` : window.location.origin;
      setFrameOrigin(new URL(base).origin);
      setSrc(`${base}/plugins/${encodeURIComponent(pluginId)}/gui/?host=${encodeURIComponent(window.location.origin)}`);
    });
    return () => {
      alive = false;
    };
  }, [pluginId]);

  // Drop the stream subscription with the panel, so closed panels stop the forwarding.
  useEffect(() => {
    const subId = subIdRef.current!;
    return () => {
      if (subscribedRef.current) {
        conn.unsubscribe(subId);
        subscribedRef.current = false;
      }
      removePluginPatterns(pluginId);
    };
  }, [conn, pluginId]);

  const requireScope = useCallback(
    (scope: string): Promise<void> => {
      detailRef.current ??= getPlugin(pluginId);
      return detailRef.current.then((d) => {
        if (!(d.scopes ?? []).includes(scope)) {
          throw new Error(`plugin did not declare the '${scope}' scope`);
        }
      });
    },
    [pluginId],
  );

  const onMessage = useCallback(
    (ev: MessageEvent) => {
      if (!frameOrigin || ev.origin !== frameOrigin) return;
      const data = ev.data as BridgeMessage | undefined;
      if (!data || data.__virta !== true || data.plugin !== pluginId || !data.seq || !data.msg) return;
      const frame = iframeRef.current?.contentWindow;
      if (!frame || ev.source !== frame) return;

      const reply = (result: unknown, error?: string) => {
        frame.postMessage({ __virta_host: true, plugin: pluginId, seq: data.seq, result, error }, frameOrigin);
      };
      const errText = (e: unknown, fallback: string) => (e instanceof Error ? e.message : fallback);

      switch (data.msg.type) {
        case 'config.get':
          getPluginConfig(pluginId)
            .then((cfg) => reply(cfg))
            .catch((e) => reply(undefined, errText(e, 'config unavailable')));
          break;
        case 'config.set':
          requireScope('storage')
            .then(() => {
              const values = ((data.msg.payload ?? {}) as { values?: Record<string, unknown> }).values;
              return putPluginConfig(pluginId, values ?? {});
            })
            .then(() => reply({ ok: true }))
            .catch((e) => reply(undefined, errText(e, 'config save failed')));
          break;
        case 'http.fetch':
          pluginHttp(pluginId, (data.msg.payload ?? {}) as PluginHttpRequest)
            .then((res) => reply(res))
            .catch((e) => reply(undefined, errText(e, 'request failed')));
          break;
        case 'events.subscribe':
          requireScope('events')
            .then(() => {
              if (!subscribedRef.current) {
                const channels = ((data.msg.payload ?? {}) as { channels?: string[] }).channels;
                conn.subscribe(
                  subIdRef.current!,
                  {
                    onMessage: (m) => {
                      const f = iframeRef.current?.contentWindow;
                      f?.postMessage({ __virta_host: true, plugin: pluginId, type: 'events.message', payload: m }, frameOrigin);
                    },
                  },
                  channels,
                );
                subscribedRef.current = true;
              }
              reply({ ok: true });
            })
            .catch((e) => reply(undefined, errText(e, 'subscribe failed')));
          break;
        case 'events.unsubscribe':
          if (subscribedRef.current) {
            conn.unsubscribe(subIdRef.current!);
            subscribedRef.current = false;
          }
          reply({ ok: true });
          break;
        case 'overlay.url':
          requireScope('ui')
            .then(async () => {
              const path = ((data.msg.payload ?? {}) as { path?: string }).path ?? 'overlay.html';
              // Keep the URL inside the plugin's gui directory.
              if (path.startsWith('/') || path.includes('..') || path.includes('?') || path.includes('#')) {
                throw new Error('invalid overlay path');
              }
              const d = await discover();
              if (!d) throw new Error('daemon unreachable');
              const base = d.addr ? `http://${d.addr}` : window.location.origin;
              const cfg = await getPluginConfig(pluginId);
              let token = typeof cfg.overlay_token === 'string' ? cfg.overlay_token : '';
              if (!token) {
                // Read-only token: the overlay can watch the stream and read config, nothing more.
                // Persisted in the plugin's config so the copied URL survives restarts; revoking
                // the token in Settings → Integrations invalidates every copy.
                const minted = await mintToken(`plugin-overlay:${pluginId}`, ['read']);
                token = minted.token;
                await putPluginConfig(pluginId, { ...cfg, overlay_token: token });
              }
              return `${base}/plugins/${encodeURIComponent(pluginId)}/gui/${path}?token=${encodeURIComponent(token)}`;
            })
            .then((url) => reply({ url }))
            .catch((e) => reply(undefined, errText(e, 'overlay url unavailable')));
          break;
        case 'decorate.addPattern':
          requireScope('ui')
            .then(() => {
              const patterns = ((data.msg.payload ?? {}) as { patterns?: unknown[] }).patterns;
              const valid = (Array.isArray(patterns) ? patterns : []).filter(
                (p): p is { regex: string; type: string } =>
                  typeof (p as Record<string, unknown>)?.regex === 'string' && typeof (p as Record<string, unknown>)?.type === 'string',
              );
              setPluginPatterns(pluginId, valid);
              reply({ ok: true });
            })
            .catch((e) => reply(undefined, errText(e, 'decorate failed')));
          break;
        case 'test.relay':
          // POST the test payload to the signal relay so OBS overlay browser sources receive it.
          discover()
            .then((d) => {
              if (!d) throw new Error('daemon unreachable');
              const base = d.addr ? `http://${d.addr}` : window.location.origin;
              return fetch(`${base}/v1/plugins/${encodeURIComponent(pluginId)}/signal`, {
                method: 'POST',
                headers: { Authorization: `Bearer ${d.token}`, 'Content-Type': 'application/json' },
                body: JSON.stringify(data.msg.payload ?? {}),
              });
            })
            .then(() => reply({ ok: true }))
            .catch((e) => reply(undefined, errText(e, 'test relay failed')));
          break;
        default:
          reply(undefined, `unknown message type: ${data.msg.type}`);
      }
    },
    [pluginId, frameOrigin, conn, requireScope],
  );

  useEffect(() => {
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [onMessage]);

  if (unreachable) {
    return (
      <div className={styles.placeholder}>
        <span className={styles.label}>{pluginId}</span>
        <span className={styles.hint}>Daemon not reachable — plugin panels need a running virtad.</span>
      </div>
    );
  }
  if (!src) return <div className={styles.placeholder} aria-busy="true" />;

  return (
    <iframe
      ref={iframeRef}
      src={src}
      title={`Plugin: ${pluginId}`}
      sandbox="allow-scripts allow-same-origin"
      loading="lazy"
      style={{ display: 'block', width: '100%', height: '100%', border: 0, background: 'transparent' }}
    />
  );
}
