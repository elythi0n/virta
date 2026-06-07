import { useEffect, useState } from 'react';

// In Wails v3 the runtime sets window.wails. In v2 it set window.go.
// Both are absent in a plain browser tab, so checking either detects "running inside Wails".
// This is intentionally true for both the native GTK window AND the wails dev browser relay —
// the window controls work in both and there is no reason to distinguish them.
declare global {
  interface Window {
    // Wails v3 runtime (window.wails = Runtime from @wailsio/runtime)
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
    // Wails v2 legacy (kept so old code compiles; window.go is absent in v3)
    go?: {
      main?: {
        App?: Record<string, unknown>;
      };
    };
  }
}

export function useIsDesktop(): boolean {
  const [desktop, setDesktop] = useState(false);
  useEffect(() => {
    // window.wails is injected by Wails v3; window.go was injected by Wails v2.
    setDesktop(
      typeof window !== 'undefined' &&
      (!!window.wails || !!window.go?.main?.App),
    );
  }, []);
  return desktop;
}
