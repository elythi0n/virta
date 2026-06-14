import { useCallback, useRef, type CSSProperties, type PointerEvent as ReactPointerEvent } from 'react';
import { DEFAULT_OVERLAY, type ChatLine, type OverlayConfig, type OverlayRect } from './types';
import styles from './ChatOverlay.module.css';

const FONT_STACK: Record<OverlayConfig['font'], string> = {
  ui: 'var(--virta-font-ui)',
  mono: 'var(--virta-font-mono)',
  rounded: '"Nunito", "Quicksand", ui-rounded, var(--virta-font-ui)',
  condensed: '"Roboto Condensed", "Oswald", var(--virta-font-ui)',
};

const CORNERS = ['nw', 'ne', 'sw', 'se'] as const;
type Corner = (typeof CORNERS)[number];
const MIN = 0.08; // smallest box edge as a fraction of the stage

function clamp01(n: number) {
  return Math.max(0, Math.min(1, n));
}

type Props = {
  lines: ChatLine[];
  config: OverlayConfig;
  /** Non-animated static frame (paused / composition). */
  staticFrame?: boolean;
  /** Editable mode: the box can be dragged and resized; emits rect changes. */
  editable?: boolean;
  onRectChange?: (rect: OverlayRect) => void;
};

// The chat overlay exactly as it bakes into a clip. In editable mode it becomes a draggable,
// corner-resizable box over the video; otherwise it's pure presentation (used for export frames).
export default function ChatOverlay({ lines, config, staticFrame, editable, onRectChange }: Props) {
  const boxRef = useRef<HTMLDivElement>(null);
  const rect = config.rect ?? DEFAULT_OVERLAY.rect;

  // Convert a pointer drag into rect-space (fractions of the stage) and apply move/resize.
  const startGesture = useCallback(
    (mode: 'move' | Corner) => (e: ReactPointerEvent) => {
      if (!editable || !onRectChange) return;
      e.preventDefault();
      e.stopPropagation();
      const parent = boxRef.current?.offsetParent as HTMLElement | null;
      const pr = parent?.getBoundingClientRect();
      if (!pr) return;
      const start = { ...rect };
      const sx = e.clientX;
      const sy = e.clientY;
      // Capture the pointer so the drag keeps tracking even as it passes over the video iframe.
      const captureEl = e.currentTarget as HTMLElement;
      const pointerId = e.pointerId;
      try {
        captureEl.setPointerCapture(pointerId);
      } catch {
        /* not all targets support capture */
      }

      const move = (ev: PointerEvent) => {
        const dx = (ev.clientX - sx) / pr.width;
        const dy = (ev.clientY - sy) / pr.height;
        if (mode === 'move') {
          onRectChange({
            ...start,
            x: clamp01(Math.min(start.x + dx, 1 - start.w)),
            y: clamp01(Math.min(start.y + dy, 1 - start.h)),
          });
          return;
        }
        let { x, y, w, h } = start;
        if (mode === 'ne' || mode === 'se') w = Math.max(MIN, Math.min(start.w + dx, 1 - start.x));
        if (mode === 'sw' || mode === 'se') h = Math.max(MIN, Math.min(start.h + dy, 1 - start.y));
        if (mode === 'nw' || mode === 'sw') {
          const right = start.x + start.w;
          x = clamp01(Math.min(start.x + dx, right - MIN));
          w = right - x;
        }
        if (mode === 'nw' || mode === 'ne') {
          const bottom = start.y + start.h;
          y = clamp01(Math.min(start.y + dy, bottom - MIN));
          h = bottom - y;
        }
        onRectChange({ x, y, w, h });
      };
      const up = () => {
        try {
          captureEl.releasePointerCapture(pointerId);
        } catch {
          /* ignore */
        }
        window.removeEventListener('pointermove', move);
        window.removeEventListener('pointerup', up);
      };
      window.addEventListener('pointermove', move);
      window.addEventListener('pointerup', up);
    },
    [editable, onRectChange, rect],
  );

  if (!config.enabled) return null;
  const shown = lines.slice(-config.maxLines);

  const style: CSSProperties = {
    left: `${rect.x * 100}%`,
    top: `${rect.y * 100}%`,
    width: `${rect.w * 100}%`,
    height: `${rect.h * 100}%`,
    padding: `${Math.round(10 * config.padding)}px`,
    fontFamily: FONT_STACK[config.font],
    // Inline so it beats any class-order ambiguity: the editable box must catch clicks (not the
    // video underneath), while a non-editable frame stays click-through.
    pointerEvents: editable ? 'auto' : 'none',
    ['--ov-bg-alpha' as string]: String(config.opacity),
    ['--ov-scale' as string]: String(config.scale),
    ['--ov-fade' as string]: `${config.fadeMs}ms`,
  };

  return (
    <div
      ref={boxRef}
      className={`${styles.overlay} ${styles[`theme-${config.theme}`]} ${config.shadow ? styles.shadow : ''} ${editable ? styles.editable : ''}`}
      style={style}
      onPointerDown={editable ? startGesture('move') : undefined}
    >
      <div className={styles.lines}>
        {shown.map((l) => (
          <div key={l.id} className={`${styles.line} ${staticFrame ? styles.noAnim : ''}`}>
            {config.showTimestamps && <span className={styles.time}>{formatOffset(l.offset)}</span>}
            {config.nameStyle !== 'hidden' && (
              <span
                className={`${styles.name} ${styles[`name-${config.nameStyle}`]}`}
                style={config.nameStyle === 'colored' ? { color: l.color || config.accent } : undefined}
              >
                {l.author}
              </span>
            )}
            <span className={styles.body}>{l.text}</span>
          </div>
        ))}
      </div>
      {editable &&
        CORNERS.map((c) => (
          <span key={c} className={`${styles.handle} ${styles[`handle-${c}`]}`} onPointerDown={startGesture(c)} />
        ))}
    </div>
  );
}

function formatOffset(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, '0');
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`;
}
