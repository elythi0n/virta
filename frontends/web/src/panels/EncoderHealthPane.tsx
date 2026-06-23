import { useEffect, useRef, useState } from 'react';
import { getOBSStreamStatus, type OBSStreamStatus } from '../daemon/obsws';
import styles from './EncoderHealthPane.module.css';

// EncoderHealthPane: tiny live monitor for the broadcast itself — uptime + computed bitrate from
// the delta of bytes_sent across polls. OBS's WS only exposes cumulative bytes, so we keep one
// previous sample in a ref and divide by the elapsed time. A reconnect badge surfaces when the
// stream is dropping; a single number per metric beats a busy graph for at-a-glance reading.
const POLL_MS = 1500;

interface Sample {
  bytes: number;
  durationMs: number;
}

export default function EncoderHealthPane() {
  const [stream, setStream] = useState<OBSStreamStatus | null>(null);
  const [bitrateKbps, setBitrateKbps] = useState<number | null>(null);
  const prev = useRef<Sample | null>(null);

  useEffect(() => {
    let alive = true;
    const poll = async () => {
      try {
        const ss = await getOBSStreamStatus();
        if (!alive) return;
        setStream(ss);
        if (ss.active && ss.bytes_sent !== undefined && ss.duration_ms !== undefined) {
          const cur: Sample = { bytes: ss.bytes_sent, durationMs: ss.duration_ms };
          const last = prev.current;
          if (last && cur.durationMs > last.durationMs && cur.bytes >= last.bytes) {
            const deltaBytes = cur.bytes - last.bytes;
            const deltaSecs = (cur.durationMs - last.durationMs) / 1000;
            if (deltaSecs > 0) setBitrateKbps((deltaBytes * 8) / 1000 / deltaSecs);
          }
          prev.current = cur;
        } else {
          prev.current = null;
          setBitrateKbps(null);
        }
      } catch {
        if (alive) {
          setStream(null);
          setBitrateKbps(null);
        }
      }
    };
    void poll();
    const id = window.setInterval(poll, POLL_MS);
    return () => {
      alive = false;
      window.clearInterval(id);
    };
  }, []);

  if (!stream || !stream.active) {
    return (
      <div className={styles.pane}>
        <div className={styles.idle}>
          <div className={styles.idleTitle}>Not streaming</div>
          <div className={styles.idleHint}>
            Encoder stats appear here while OBS is broadcasting.
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.pane}>
      <div className={styles.metric}>
        <div className={styles.metricLabel}>Uptime</div>
        <div className={styles.metricValue}>{formatDuration(stream.duration_ms ?? 0)}</div>
      </div>
      <div className={styles.metric}>
        <div className={styles.metricLabel}>Bitrate</div>
        <div className={styles.metricValue}>
          {bitrateKbps === null ? '—' : formatBitrate(bitrateKbps)}
        </div>
        <div className={styles.metricUnit}>
          {bitrateKbps === null ? 'sampling…' : 'kbps avg'}
        </div>
      </div>
      <div className={styles.metric}>
        <div className={styles.metricLabel}>Sent</div>
        <div className={styles.metricValue}>{formatBytes(stream.bytes_sent ?? 0)}</div>
      </div>
      {stream.reconnecting && (
        <div className={styles.warn}>Reconnecting to ingest…</div>
      )}
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
function formatBitrate(kbps: number): string {
  if (kbps >= 1000) return `${(kbps / 1000).toFixed(2)}M`;
  return kbps.toFixed(0);
}
function formatBytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}
