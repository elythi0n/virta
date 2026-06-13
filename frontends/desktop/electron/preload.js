'use strict';

// Preload bridge: exposes a `window.wails`-shaped API so the existing web UI (built for the Wails
// shell) runs unchanged under Electron. useIsDesktop() detects the desktop build by the presence of
// window.wails; the other methods map to IPC handlers in the main process.

const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('wails', {
  Window: {
    Minimise: () => ipcRenderer.invoke('win:minimise'),
    ToggleMaximise: () => ipcRenderer.invoke('win:toggleMaximise'),
    OpenDevTools: () => ipcRenderer.invoke('win:openDevTools'),
  },
  Application: {
    Quit: () => ipcRenderer.invoke('app:quit'),
  },
  Browser: {
    OpenURL: (url) => ipcRenderer.invoke('browser:openExternal', url),
  },
  // Bound-method call shim. The web UI calls window.wails.Call({ methodName, args }); the main
  // process dispatches by methodName (currently main.App.OpenStreamWindow).
  Call: (opts) => ipcRenderer.invoke('call', opts),
});
