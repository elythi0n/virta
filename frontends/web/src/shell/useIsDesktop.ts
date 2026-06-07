import { useEffect, useState } from 'react';

// Wails injects window.go with bound Go methods when running inside the desktop shell.
// This hook returns true only in that context; false in any plain browser.
declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          WindowMinimise?(): Promise<void>;
          WindowToggleMaximise?(): Promise<void>;
          WindowClose?(): Promise<void>;
          OpenInspector?(): Promise<void>;
        };
      };
    };
  }
}

export function useIsDesktop(): boolean {
  const [desktop, setDesktop] = useState(false);
  useEffect(() => {
    setDesktop(typeof window !== 'undefined' && !!window.go?.main?.App);
  }, []);
  return desktop;
}
