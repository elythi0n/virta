import { createContext, useContext } from 'react';

// Appearance mode: follow the OS, or pin light/dark. `theme` is the resolved token-theme name
// (what's actually applied), derived from the mode and the OS preference.
export type ThemeMode = 'system' | 'light' | 'dark';

type ThemeContextValue = {
  mode: ThemeMode;
  setMode: (mode: ThemeMode) => void;
  theme: string;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

export const ThemeProvider = ThemeContext.Provider;

// The dark/light token themes a mode resolves to (a configurable pairing is a later step).
export const DARK_THEME = 'graphite-dark';
export const LIGHT_THEME = 'light';

// Theme lives at the app root but is consumed deep in dock panels (e.g. Settings). dockview
// renders panels through React portals, which preserve context, so this reaches them.
export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider');
  return ctx;
}
