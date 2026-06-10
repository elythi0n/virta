import { useCallback, useEffect, useRef, useState } from 'react';
import { discover } from '../daemon/discovery';
import { getPluginConfig, pluginHttp, type PluginHttpRequest } from '../daemon/plugins';
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
 *   config.get            → GET  /v1/plugins/{id}/config
 *   http.fetch {url,...}  → POST /v1/plugins/{id}/http (manifest-declared endpoints only)
 *
 * Origins are pinned on both sides: the iframe only talks to our origin (passed as ?host=), and
 * we only accept messages from the daemon origin serving the iframe.
 */
export default function PluginPanel({ pluginId }: { pluginId: string }) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [src, setSrc] = useState<string | null>(null);
  const [frameOrigin, setFrameOrigin] = useState<string | null>(null);
  const [unreachable, setUnreachable] = useState(false);

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

      switch (data.msg.type) {
        case 'config.get':
          getPluginConfig(pluginId)
            .then((cfg) => reply(cfg))
            .catch((e) => reply(undefined, e instanceof Error ? e.message : 'config unavailable'));
          break;
        case 'http.fetch':
          pluginHttp(pluginId, (data.msg.payload ?? {}) as PluginHttpRequest)
            .then((res) => reply(res))
            .catch((e) => reply(undefined, e instanceof Error ? e.message : 'request failed'));
          break;
        default:
          reply(undefined, `unknown message type: ${data.msg.type}`);
      }
    },
    [pluginId, frameOrigin],
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
