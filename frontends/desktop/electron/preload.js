'use strict';

// Preload bridge: exposes window.virta — the desktop shell's API surface for the web UI. Its
// presence is how the UI detects it's running in the desktop app (vs a plain browser); the methods
// map to IPC handlers in the main process.

const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('virta', {
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
  // Open a native pop-out player window for a channel (used for YouTube and the detached player).
  OpenStreamWindow: (platform, slug) => ipcRenderer.invoke('streams:open', platform, slug),
});
