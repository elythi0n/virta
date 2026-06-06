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
        res.end(JSON.stringify({ addr: req.headers.host, token: DEV_TOKEN }));
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), devDaemonBridge()],
  // The ui-kit/feed-core workspace packages share this app's React; dedupe avoids a second copy
  // (which would break Radix's hooks/context).
  resolve: { dedupe: ['react', 'react-dom'] },
  server: {
    // fs.allow opens the sibling ui-kit/ so the dev server can serve the shared tokens.css.
    fs: { allow: ['..'] },
    proxy: DAEMON ? { '/v1': { target: DAEMON, changeOrigin: true, ws: true } } : undefined,
  },
});
