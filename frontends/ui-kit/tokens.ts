// Generated from tokens.json by cmd/tokengen. Do not edit by hand.

export const fonts = { ui: "Geist Variable", mono: "Geist Mono Variable" } as const;

export const space = [2, 4, 8, 12, 16, 24, 32] as const;

export const radius = { lg: 8, md: 6, sm: 5 } as const;

export const color = {
  "accent": "var(--virta-accent)",
  "bg-0": "var(--virta-bg-0)",
  "bg-1": "var(--virta-bg-1)",
  "bg-2": "var(--virta-bg-2)",
  "danger": "var(--virta-danger)",
  "highlight-bg": "var(--virta-highlight-bg)",
  "highlight-rail": "var(--virta-highlight-rail)",
  "line": "var(--virta-line)",
  "ok": "var(--virta-ok)",
  "plat-x": "var(--virta-plat-x)",
  "scrollbar-thumb": "var(--virta-scrollbar-thumb)",
  "scrollbar-thumb-hover": "var(--virta-scrollbar-thumb-hover)",
  "text-0": "var(--virta-text-0)",
  "text-1": "var(--virta-text-1)",
  "text-2": "var(--virta-text-2)",
  "warn": "var(--virta-warn)",
  "plat-kick": "var(--virta-plat-kick)",
  "plat-twitch": "var(--virta-plat-twitch)",
} as const;

export type ThemeName = "graphite-dark" | "light";
