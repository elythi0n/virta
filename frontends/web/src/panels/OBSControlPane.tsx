import { useCallback, useEffect, useState } from 'react';
import Icon from '../Icon';
import {
  getOBSScenes,
  getOBSStatus,
  getOBSStreamStatus,
  setOBSScene,
  startOBSStream,
  stopOBSStream,
  type OBSSceneList,
  type OBSStatus,
  type OBSStreamStatus,
} from '../daemon/obsws';
import styles from './OBSControlPane.module.css';

// OBSControlPane: the broadcaster's "big red button" surface. One big start/stop control plus a
// row of scene chips for quick switching. The richer configuration (overlays, data mappings,
// event rules) lives in the existing OBS settings pane — this one is built for live use.
//
// Polls every 2s while mounted so the duration ticks and the streaming/connected pill stays
// honest, without spinning a websocket for what is fundamentally low-frequency state.
const POLL_MS = 2000;

export default function OBSControlPane() {
  const [status, setStatus] = useState<OBSStatus | null>(null);
  const [stream, setStream] = useState<OBSStreamStatus | null>(null);
  const [scenes, setScenes] = useState<OBSSceneList | null>(null);
  const [busy, setBusy] = useState<'start' | 'stop' | null>(null);
  const [switching, setSwitching] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    const poll = async () => {
      try {
        const s = await getOBSStatus();
        if (!alive) return;
        setStatus(s);
        if (s.state === 'connected') {
          const [ss, sl] = await Promise.allSettled([getOBSStreamStatus(), getOBSScenes()]);
          if (!alive) return;
          if (ss.status === 'fulfilled') setStream(ss.value);
          if (sl.status === 'fulfilled') setScenes(sl.value);
        } else {
          setStream(null);
          setScenes(null);
        }
      } catch {
        if (alive) setStatus(null);
      }
    };
    void poll();
    const id = window.setInterval(poll, POLL_MS);
    return () => {
      alive = false;
      window.clearInterval(id);
    };
  }, []);

  const onStart = useCallback(async () => {
    setBusy('start');
    setError(null);
    try {
      await startOBSStream();
      setStream(await getOBSStreamStatus());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'start failed');
    } finally {
      setBusy(null);
    }
  }, []);

  const onStop = useCallback(async () => {
    setBusy('stop');
    setError(null);
    try {
      await stopOBSStream();
      setStream(await getOBSStreamStatus());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'stop failed');
    } finally {
      setBusy(null);
    }
  }, []);

  const onSwitch = useCallback(async (name: string) => {
    setSwitching(name);
    setError(null);
    try {
      await setOBSScene(name);
      setScenes(await getOBSScenes());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'scene switch failed');
    } finally {
      setSwitching(null);
    }
  }, []);

  const connected = status?.state === 'connected';
  const streaming = stream?.active === true;
  const duration = stream?.duration_ms;

  if (!status || status.state === 'disconnected' || status.state === 'error') {
    return (
      <div className={styles.pane}>
        <div className={styles.emptyState}>
          <div className={styles.emptyTitle}>OBS isn't connected</div>
          <div className={styles.emptyHint}>
            Open Settings → OBS to connect to a local OBS WebSocket. Once it's online the start
            button and scene switcher light up here.
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.pane}>
      <div className={styles.statusRow}>
        <span className={`${styles.statusPill} ${streaming ? styles.statusLive : styles.statusReady}`}>
          <span className={styles.statusDot} />
          {streaming ? 'Live' : 'Ready'}
        </span>
        {streaming && duration !== undefined && (
          <span className={styles.duration}>{formatDuration(duration)}</span>
        )}
        {status.obs_version && (
          <span className={styles.version}>OBS {status.obs_version}</span>
        )}
      </div>

      <button
        type="button"
        className={`${styles.bigButton} ${streaming ? styles.bigStop : styles.bigStart}`}
        disabled={!connected || busy !== null}
        onClick={streaming ? onStop : onStart}
      >
        <Icon name={streaming ? 'pause' : 'play'} size={22} />
        <span>{busy === 'start' ? 'Starting…' : busy === 'stop' ? 'Stopping…' : streaming ? 'Stop streaming' : 'Start streaming'}</span>
      </button>

      {error && <div className={styles.error}>{error}</div>}

      <div className={styles.section}>
        <div className={styles.sectionHead}>Scenes</div>
        {scenes && scenes.scenes.length > 0 ? (
          <div className={styles.sceneGrid}>
            {scenes.scenes.map((name) => {
              const active = name === scenes.current;
              const isSwitching = name === switching;
              return (
                <button
                  key={name}
                  type="button"
                  className={`${styles.scene} ${active ? styles.sceneActive : ''}`}
                  onClick={() => onSwitch(name)}
                  disabled={!!switching || active}
                >
                  <span className={styles.sceneName}>{name}</span>
                  {active && <span className={styles.sceneBadge}>Live</span>}
                  {isSwitching && <span className={styles.spinner} aria-hidden="true" />}
                </button>
              );
            })}
          </div>
        ) : (
          <div className={styles.sceneEmpty}>No scenes reported.</div>
        )}
      </div>
    </div>
  );
}

function formatDuration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}:${pad(m)}:${pad(s)}`;
  return `${m}:${pad(s)}`;
}
function pad(n: number): string {
  return n < 10 ? `0${n}` : String(n);
}
