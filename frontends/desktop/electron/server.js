'use strict';

// Local renderer server: a loopback HTTP server that serves the built web UI plus the three shell
// endpoints the SPA fetches (/__discovery, /__deeplink, /__integration). Serving over
// http://localhost (rather than file://) gives the renderer a real origin, which is what lets the
// Twitch/Kick player iframes embed inline — they accept `parent=localhost`. The web build's API
// calls go cross-origin to the daemon via the discovery address; the daemon's CORS already trusts
// loopback origins.

const http = require('node:http');
const fsp = require('node:fs/promises');
const path = require('node:path');
const { app } = require('electron');

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.map': 'application/json; charset=utf-8',
  '.webmanifest': 'application/manifest+json',
  '.wasm': 'application/wasm',
};

// webRoot points at the bundled web build (inside the asar when packaged; the staging dir in dev).
function webRoot() {
  return path.join(__dirname, '..', 'resources', 'web');
}

async function sendFile(res, filePath) {
  const data = await fsp.readFile(filePath);
  res.statusCode = 200;
  res.setHeader('Content-Type', MIME[path.extname(filePath).toLowerCase()] || 'application/octet-stream');
  res.end(data);
}

async function serveStatic(req, res, root) {
  let pathname = decodeURIComponent(new URL(req.url, 'http://localhost').pathname);
  if (pathname === '/' || pathname === '') pathname = '/index.html';
  const filePath = path.normalize(path.join(root, pathname));
  // Reject path traversal outside the web root.
  if (filePath !== root && !filePath.startsWith(root + path.sep)) {
    res.statusCode = 403;
    return res.end('forbidden');
  }
  try {
    return await sendFile(res, filePath);
  } catch {
    // SPA fallback: client-routed paths (no file extension) fall back to index.html.
    if (!path.extname(pathname)) {
      try {
        return await sendFile(res, path.join(root, 'index.html'));
      } catch {
        /* index missing — fall through to 404 */
      }
    }
    res.statusCode = 404;
    return res.end('not found');
  }
}

// startServer binds a loopback HTTP server on a random free port and resolves with { server, port }.
// Callbacks supply live shell state: discovery (daemon addr/token), the pending deep link, and the
// integration report.
function startServer({ getDiscovery, takeDeepLink, getIntegration }) {
  const root = webRoot();
  const server = http.createServer(async (req, res) => {
    const { pathname } = new URL(req.url, 'http://localhost');
    // The bearer token rides as a query param on WS handshakes elsewhere; keep referrers off.
    res.setHeader('Referrer-Policy', 'no-referrer');

    if (pathname === '/__discovery') {
      const d = getDiscovery();
      if (!d) {
        res.statusCode = 503;
        return res.end('daemon not ready');
      }
      res.setHeader('Content-Type', 'application/json');
      return res.end(JSON.stringify(d));
    }
    if (pathname === '/__deeplink') {
      res.setHeader('Content-Type', 'application/json');
      return res.end(JSON.stringify({ url: takeDeepLink() }));
    }
    if (pathname === '/__integration') {
      res.setHeader('Content-Type', 'application/json');
      return res.end(JSON.stringify(getIntegration()));
    }
    return serveStatic(req, res, root);
  });

  return new Promise((resolve, reject) => {
    server.once('error', reject);
    // Bind loopback only; navigate via http://localhost:<port> so player iframes see parent=localhost.
    server.listen(0, '127.0.0.1', () => {
      resolve({ server, port: server.address().port });
    });
  });
}

module.exports = { startServer };
