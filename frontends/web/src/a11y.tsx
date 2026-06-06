import { createContext, useContext } from 'react';

type A11yValue = {
  // Force-disable transitions/animations regardless of the OS preference.
  reduceMotion: boolean;
  setReduceMotion: (v: boolean) => void;
  // Swap the UI typeface for the bundled dyslexia-friendly font (OpenDyslexic).
  dyslexicFont: boolean;
  setDyslexicFont: (v: boolean) => void;
};

const A11yContext = createContext<A11yValue>({
  reduceMotion: false,
  setReduceMotion: () => {},
  dyslexicFont: false,
  setDyslexicFont: () => {},
});

export const A11yProvider = A11yContext.Provider;

// Accessibility preferences live at the app root (they toggle document-level attributes) and are
// read by Settings.
export function useA11y(): A11yValue {
  return useContext(A11yContext);
}
