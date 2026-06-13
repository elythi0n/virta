import { defineConfig, type PluginOption } from 'vite';
import react from '@vitejs/plugin-react';

// Point `npm run dev` at a locally running virtad for fast iteration with real data:
//   VIRTA_DAEMON=http://127.0.0.1:8799 VIRTA_TOKEN=dev npm run dev
// and run the daemon with a matching fixed address + token:
//   VIRTA_ADDR=127.0.0.1:8799 VIRTA_TOKEN=dev ./dist/virtad
// The bridge serves /__discovery pointing back at this origin and proxies /v1 (incl. the WS) to
// the daemon, so the SPA talks to it same-origin (no CORS). Without VIRTA_DAEMON, dev runs UI-only
// (synthetic feed, Sources shows "not connected") exactly as before.
const DAEMON = process.env.VIRTA_DAEMON;
const DEV_TOKEN = process.env.VIRTA_TOKEN ?? '';

function devDaemonBridge(): PluginOption {
  return {
    name: 'virta-dev-daemon-bridge',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use('/__discovery', (req, res) => {
        if (!DAEMON) {
          res.statusCode = 404;
          res.end();
          return;
        }
        res.setHeader('content-type', 'application/json');
        // Empty addr → the SPA uses this same origin, where the /v1 proxy below fronts the daemon.
        res.end(JSON.stringify({ addr: '', token: DEV_TOKEN }));
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), devDaemonBridge()],
  // The ui-kit/feed-core workspace packages share this app's React; dedupe avoids a second copy
  // (which would break Radix's hooks/context).
  resolve: { dedupe: ['react', 'react-dom'] },
  // Multi-page: the main app (/) and the transparent overlay (/overlay.html → served by virtad
  // at /overlay). Both share the same ui-kit/feed-core packages.
  build: {
    rollupOptions: {
      input: { app: 'index.html', overlay: 'overlay.html' },
    },
  },
  server: {
    // fs.allow opens the sibling ui-kit/ so the dev server can serve the shared tokens.css.
    fs: { allow: ['..'] },
    proxy: DAEMON
      ? {
          '/v1': { target: DAEMON, changeOrigin: true, ws: true },
          // Plugin GUI assets are served by the daemon, not Vite. Without this proxy,
          // /plugins/{id}/gui/ hits Vite's SPA catch-all and loads the host app in the iframe.
          '/plugins': { target: DAEMON, changeOrigin: true },
        }
      : undefined,
  },
});
