import { useCallback, useRef, useState } from 'react';
import { Tooltip } from '@virta/ui-kit';
import { clock } from './format';
import type { TimelineMoment } from './types';
import styles from './Timeline.module.css';

type Props = {
  duration: number;
  current: number;
  moments: TimelineMoment[];
  inSec: number;
  outSec: number;
  onSeek: (sec: number) => void;
  onSetIn: (sec: number) => void;
  onSetOut: (sec: number) => void;
};

function clamp(n: number, lo: number, hi: number) {
  return Math.max(lo, Math.min(hi, n));
}

// The scrub bar. Click or drag anywhere to seek; scroll the wheel over it to nudge forward/back; the
// moments heat layer, the selected clip range, draggable in/out handles, and a hover preview all
// sit on the same track. Positions are fractions of the duration, so it's resolution-independent.
export default function Timeline({ duration, current, moments, inSec, outSec, onSeek, onSetIn, onSetOut }: Props) {
  const trackRef = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const [scrubbing, setScrubbing] = useState(false);
  const dur = Math.max(duration, 1);

  const fracFromX = useCallback((clientX: number) => {
    const el = trackRef.current;
    if (!el) return 0;
    const r = el.getBoundingClientRect();
    return clamp((clientX - r.left) / r.width, 0, 1);
  }, []);

  const beginDrag = useCallback(
    (kind: 'seek' | 'in' | 'out') => (e: React.PointerEvent) => {
      e.preventDefault();
      if (kind === 'seek') setScrubbing(true);
      const apply = (clientX: number) => {
        const sec = fracFromX(clientX) * dur;
        if (kind === 'seek') onSeek(sec);
        else if (kind === 'in') onSetIn(clamp(sec, 0, outSec - 0.5));
        else onSetOut(clamp(sec, inSec + 0.5, dur));
      };
      apply(e.clientX);
      const move = (ev: PointerEvent) => apply(ev.clientX);
      const up = () => {
        setScrubbing(false);
        window.removeEventListener('pointermove', move);
        window.removeEventListener('pointerup', up);
      };
      window.addEventListener('pointermove', move);
      window.addEventListener('pointerup', up);
    },
    [dur, fracFromX, inSec, outSec, onSeek, onSetIn, onSetOut],
  );

  const onWheel = useCallback(
    (e: React.WheelEvent) => {
      const raw = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY;
      const step = e.shiftKey ? 15 : 5;
      onSeek(clamp(current + (raw > 0 ? step : -step), 0, dur));
    },
    [current, dur, onSeek],
  );

  const pct = (sec: number) => `${(clamp(sec, 0, dur) / dur) * 100}%`;

  return (
    <div className={styles.timeline}>
      <div
        ref={trackRef}
        className={`${styles.track} ${scrubbing ? styles.scrubbing : ''}`}
        onPointerDown={beginDrag('seek')}
        onWheel={onWheel}
        onMouseMove={(e) => setHover(fracFromX(e.clientX) * dur)}
        onMouseLeave={() => setHover(null)}
      >
        {/* Played progress */}
        <div className={styles.played} style={{ width: pct(current) }} />

        {/* Moment heat markers */}
        {moments.map((m) => (
          <Tooltip key={m.id} content={`${Math.round(m.peakRate)} msg/s spike`} side="top">
            <div className={styles.moment} style={{ left: pct(m.startSec), width: `max(3px, ${((m.endSec - m.startSec) / dur) * 100}%)` }} />
          </Tooltip>
        ))}

        {/* Selected clip range */}
        <div className={styles.range} style={{ left: pct(inSec), right: `calc(100% - ${pct(outSec)})` }} />

        {/* Hover preview */}
        {hover != null && !scrubbing && (
          <>
            <div className={styles.hoverLine} style={{ left: pct(hover) }} />
            <div className={styles.hoverLabel} style={{ left: pct(hover) }}>{clock(hover)}</div>
          </>
        )}

        {/* Playhead */}
        <div className={`${styles.playhead} ${scrubbing ? styles.noGlide : ''}`} style={{ left: pct(current) }} />

        {/* In / out handles */}
        <div className={`${styles.handle} ${styles.inHandle}`} style={{ left: pct(inSec) }} onPointerDown={(e) => { e.stopPropagation(); beginDrag('in')(e); }} role="slider" aria-label="Clip start" tabIndex={0} />
        <div className={`${styles.handle} ${styles.outHandle}`} style={{ left: pct(outSec) }} onPointerDown={(e) => { e.stopPropagation(); beginDrag('out')(e); }} role="slider" aria-label="Clip end" tabIndex={0} />
      </div>
    </div>
  );
}
