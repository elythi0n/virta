import { useEffect, useState } from 'react';

// The Electron desktop shell's preload exposes window.wails (a compatibility-shaped bridge kept so
// this UI runs unchanged from its Wails origins). It is absent in a plain browser tab, so its
// presence detects "running inside the desktop app". window.go is legacy and never set anymore.
declare global {
  interface Window {
    // Desktop shell bridge (preload contextBridge). Method shape preserved from the Wails runtime.
    wails?: {
      Window?: {
        Minimise?():        Promise<void>;
        ToggleMaximise?():  Promise<void>;
        OpenDevTools?():    Promise<void>;
      };
      Application?: {
        Quit?(): Promise<void>;
      };
      Browser?: {
        OpenURL?(url: string): Promise<void>;
      };
      Call?(opts: { methodName: string; args: unknown[] }): Promise<unknown>;
    };
  }
}

export function useIsDesktop(): boolean {
  const [desktop, setDesktop] = useState(false);
  useEffect(() => {
    // The desktop shell's preload exposes window.wails; a plain browser has no such global.
    setDesktop(typeof window !== 'undefined' && !!window.wails);
  }, []);
  return desktop;
}
