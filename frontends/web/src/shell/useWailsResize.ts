import { useEffect } from 'react';

// Threshold in px from the window edge that activates a resize cursor.
const EDGE = 6;

type EdgeName =
  | 'se-resize' | 'sw-resize' | 'ne-resize' | 'nw-resize'
  | 'e-resize'  | 'w-resize'  | 'n-resize'  | 's-resize';

function getEdge(x: number, y: number, w: number, h: number): EdgeName | null {
  const right  = x > w - EDGE;
  const left   = x < EDGE;
  const bottom = y > h - EDGE;
  const top    = y < EDGE;

  if (right  && bottom) return 'se-resize';
  if (left   && bottom) return 'sw-resize';
  if (right  && top)    return 'ne-resize';
  if (left   && top)    return 'nw-resize';
  if (right)            return 'e-resize';
  if (left)             return 'w-resize';
  if (bottom)           return 's-resize';
  if (top)              return 'n-resize';
  return null;
}

// Adds window-edge resize detection for frameless Wails v3 windows on Linux.
// Wails v3's drag.ts has a !IsWindows() gate that skips edge detection on Linux,
// but gtk_window_begin_resize_drag IS implemented. We trigger it ourselves by
// sending wails:resize:<edge> via window._wails.invoke, which the Go handler
// picks up and calls gtk_window_begin_resize_drag.
//
// Only activates when window._wails.invoke is present (Wails webview context).
export function useWailsResize() {
  useEffect(() => {
    const inv = (window as any)._wails?.invoke;
    if (!inv) return; // plain browser — no-op

    let edge: EdgeName | null = null;

    function onMove(e: MouseEvent) {
      const w = window.innerWidth;
      const h = window.innerHeight;
      const next = getEdge(e.clientX, e.clientY, w, h);
      if (next !== edge) {
        edge = next;
        document.documentElement.style.cursor = edge || '';
      }
    }

    function onDown(e: MouseEvent) {
      if (e.button !== 0 || !edge) return;
      // Prevent the drag handler from also starting a window move.
      e.stopImmediatePropagation();
      e.preventDefault();
      inv(`wails:resize:${edge}`);
    }

    window.addEventListener('mousemove', onMove, { capture: true });
    window.addEventListener('mousedown', onDown, { capture: true });
    return () => {
      window.removeEventListener('mousemove', onMove, { capture: true });
      window.removeEventListener('mousedown', onDown, { capture: true });
    };
  }, []);
}
