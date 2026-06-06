import { createContext, useContext } from 'react';

export type ThemeName = string;

type ThemeContextValue = { theme: ThemeName; setTheme: (theme: ThemeName) => void };

const ThemeContext = createContext<ThemeContextValue | null>(null);

export const ThemeProvider = ThemeContext.Provider;

// Theme lives at the app root but is consumed deep in dock panels (e.g. Settings). dockview
// renders panels through React portals, which preserve context, so this reaches them.
export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider');
  return ctx;
}
