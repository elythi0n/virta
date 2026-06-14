import { useEffect, useState } from 'react';

// The Electron desktop shell's preload exposes window.virta. It is absent in a plain browser tab,
// so its presence detects "running inside the desktop app".
declare global {
  interface Window {
    // Desktop shell bridge (preload contextBridge → IPC to the Electron main process).
    virta?: {
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
      OpenStreamWindow?(platform: string, slug: string): Promise<void>;
      twitchGql?(body: unknown): Promise<unknown>;
    };
  }
}

export function useIsDesktop(): boolean {
  const [desktop, setDesktop] = useState(false);
  useEffect(() => {
    // The desktop shell's preload exposes window.virta; a plain browser has no such global.
    setDesktop(typeof window !== 'undefined' && !!window.virta);
  }, []);
  return desktop;
}
