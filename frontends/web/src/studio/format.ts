// Format a number of seconds as a clock: m:ss, or h:mm:ss past an hour.
export function clock(sec: number): string {
  const s = Math.max(0, Math.floor(sec || 0));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, '0');
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`;
}

// Compact duration like "0:42" / "1:05".
export function duration(sec: number): string {
  return clock(Math.max(0, Math.round(sec)));
}
