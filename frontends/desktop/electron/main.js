'use strict';

// Virta desktop shell (Electron). Replaces the former Wails v3 Go shell. Responsibilities:
//   - spawn and supervise the embedded virtad daemon (daemon.js)
//   - serve the built web UI + shell endpoints over http://localhost (server.js)
//   - host the UI in a frameless Chromium window
//   - strip stream players' frame-blocking headers so Twitch/Kick embed inline (the whole reason
//     we moved off Wails: WebView2 served the app over http://wails.localhost, which Twitch's
//     https-only frame-ancestors rejected and Wails gave no way to rewrite)
//   - bridge window controls / external-open / pop-out player to the renderer via IPC
//   - handle the virta:// deep link and single-instance behavior

const { app, BrowserWindow, ipcMain, shell, session } = require('electron');
const path = require('node:path');
const { Daemon } = require('./daemon');
const { startServer } = require('./server');
const { resolveIntegration } = require('./integration');
const { openStreamWindow } = require('./streams');

app.setName('Virta');

let mainWindow = null;
let daemon = null;
let httpServer = null;
let rendererPort = 0;
let pendingDeepLink = firstDeepLink(process.argv);

// ── Single instance ─────────────────────────────────────────────────────────
// A second launch (e.g. clicking a virta:// link) forwards its deep link to the running instance
// and exits, rather than starting a second app + daemon.
if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', (_event, argv) => {
    const link = firstDeepLink(argv);
    if (link) deliverDeepLink(link);
    if (mainWindow) {
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.focus();
    }
  });
  registerProtocol();
  bootstrap();
}

function firstDeepLink(argv) {
  return (argv || []).find((a) => typeof a === 'string' && a.startsWith('virta://')) || '';
}

function registerProtocol() {
  if (process.defaultApp) {
    // In development (`electron .`) the protocol client must point at the script explicitly.
    if (process.argv.length >= 2) {
      app.setAsDefaultProtocolClient('virta', process.execPath, [path.resolve(process.argv[1])]);
    }
  } else {
    app.setAsDefaultProtocolClient('virta');
  }
}

// macOS delivers deep links via open-url rather than argv.
app.on('open-url', (event, url) => {
  event.preventDefault();
  deliverDeepLink(url);
});

function deliverDeepLink(url) {
  pendingDeepLink = url;
  if (mainWindow) mainWindow.webContents.send('virta:deeplink', url);
}

async function bootstrap() {
  await app.whenReady();
  installPlayerHeaderRewrite();

  daemon = new Daemon();
  try {
    await daemon.start();
  } catch (err) {
    // Non-fatal: the UI still loads and shows a "not connected" state; the user can retry by
    // relaunching. Surfacing this in-app is handled by the existing discovery retry logic.
    console.error('[virta] daemon failed to start:', err && err.message ? err.message : err);
  }

  const { server, port } = await startServer({
    getDiscovery: () => (daemon ? daemon.discovery() : null),
    takeDeepLink: () => {
      const u = pendingDeepLink;
      pendingDeepLink = '';
      return u;
    },
    getIntegration: () => resolveIntegration(),
  });
  httpServer = server;
  rendererPort = port;

  createWindow();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
}

// installPlayerHeaderRewrite strips frame-blocking response headers (CSP frame-ancestors,
// X-Frame-Options) from Twitch/Kick player responses so their iframes render inside the panel.
// Scoped to those hosts only; everything else passes through untouched.
function installPlayerHeaderRewrite() {
  const isPlayerHost = (urlStr) => {
    let host = '';
    try {
      host = new URL(urlStr).hostname;
    } catch {
      return false;
    }
    return (
      host === 'player.twitch.tv' ||
      host.endsWith('.twitch.tv') ||
      host.endsWith('.ttvnw.net') ||
      host === 'kick.com' ||
      host.endsWith('.kick.com')
    );
  };

  session.defaultSession.webRequest.onHeadersReceived((details, callback) => {
    if (!isPlayerHost(details.url)) {
      return callback({});
    }
    const headers = details.responseHeaders || {};
    for (const key of Object.keys(headers)) {
      const lower = key.toLowerCase();
      if (
        lower === 'content-security-policy' ||
        lower === 'content-security-policy-report-only' ||
        lower === 'x-frame-options'
      ) {
        delete headers[key];
      }
    }
    callback({ responseHeaders: headers });
  });
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 832,
    minWidth: 960,
    minHeight: 600,
    frame: false, // custom titlebar in the web UI; -webkit-app-region: drag handles dragging
    backgroundColor: '#0e0f12',
    show: false,
    title: 'Virta',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      webviewTag: false,
    },
  });

  mainWindow.once('ready-to-show', () => mainWindow.show());
  mainWindow.loadURL(`http://localhost:${rendererPort}/`);

  // Keep navigations inside the app; send external links (and any window.open) to the OS browser.
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith(`http://localhost:${rendererPort}`)) return { action: 'allow' };
    shell.openExternal(url);
    return { action: 'deny' };
  });
  mainWindow.webContents.on('will-navigate', (event, url) => {
    if (!url.startsWith(`http://localhost:${rendererPort}`)) {
      event.preventDefault();
      shell.openExternal(url);
    }
  });

  // DevTools shortcut (F12 / Ctrl+Shift+I) — handy in any build; the titlebar also exposes it.
  mainWindow.webContents.on('before-input-event', (event, input) => {
    const toggle =
      input.key === 'F12' ||
      (input.control && input.shift && input.key.toLowerCase() === 'i');
    if (toggle && input.type === 'keyDown') {
      mainWindow.webContents.toggleDevTools();
      event.preventDefault();
    }
  });

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

// ── IPC: window controls + bound-method shim ─────────────────────────────────
ipcMain.handle('win:minimise', () => mainWindow && mainWindow.minimize());
ipcMain.handle('win:toggleMaximise', () => {
  if (!mainWindow) return;
  if (mainWindow.isMaximized()) mainWindow.unmaximize();
  else mainWindow.maximize();
});
ipcMain.handle('win:openDevTools', () => mainWindow && mainWindow.webContents.openDevTools({ mode: 'detach' }));
ipcMain.handle('app:quit', () => app.quit());
ipcMain.handle('browser:openExternal', (_event, url) => {
  if (typeof url === 'string' && /^https?:\/\//.test(url)) shell.openExternal(url);
});
ipcMain.handle('call', (_event, opts) => {
  if (opts && opts.methodName === 'main.App.OpenStreamWindow') {
    const [platform, slug] = opts.args || [];
    openStreamWindow(platform, slug);
    return { ok: true };
  }
  throw new Error(`unknown bound method: ${opts && opts.methodName}`);
});

// ── Shutdown ─────────────────────────────────────────────────────────────────
app.on('before-quit', () => {
  try { if (daemon) daemon.stop(); } catch { /* ignore */ }
  try { if (httpServer) httpServer.close(); } catch { /* ignore */ }
});
app.on('window-all-closed', () => app.quit());
