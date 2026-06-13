'use strict';

// Daemon supervisor: spawns the embedded virtad as a child, waits until it advertises itself and
// answers a health probe, then exposes its address + bearer token. Mirrors the former Go shell's
// desktop.Ensure flow (launch → read discovery file → confirm listener), but launches a fresh
// daemon every time into an isolated runtime dir so we never attach to a stale, pre-fix daemon
// (the class of bug that plagued the Wails build on Windows).

const { spawn } = require('node:child_process');
const path = require('node:path');
const fs = require('node:fs');
const fsp = require('node:fs/promises');
const http = require('node:http');
const { app } = require('electron');

const HEALTH_TIMEOUT_MS = 750;
const START_TIMEOUT_MS = 20000;
const POLL_INTERVAL_MS = 50;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// daemonBinaryPath resolves the bundled virtad: from app resources when packaged, from the staging
// dir (populated by `make app`) in development.
function daemonBinaryPath() {
  const exe = process.platform === 'win32' ? 'virtad.exe' : 'virtad';
  return app.isPackaged
    ? path.join(process.resourcesPath, 'bin', exe)
    : path.join(__dirname, '..', 'resources', 'bin', exe);
}

// probeHealth reports whether the daemon's unauthenticated health endpoint answers 200.
function probeHealth(addr) {
  const [host, port] = addr.split(':');
  return new Promise((resolve) => {
    const req = http.get({ host, port: Number(port), path: '/v1/health', timeout: HEALTH_TIMEOUT_MS }, (res) => {
      res.resume();
      resolve(res.statusCode === 200);
    });
    req.on('error', () => resolve(false));
    req.on('timeout', () => { req.destroy(); resolve(false); });
  });
}

// readDiscovery parses the daemon.json the daemon writes once its listener is bound.
async function readDiscovery(file) {
  try {
    const d = JSON.parse(await fsp.readFile(file, 'utf8'));
    if (d && typeof d.addr === 'string' && d.addr && typeof d.token === 'string' && d.token) return d;
  } catch {
    /* not written yet */
  }
  return null;
}

class Daemon {
  constructor() {
    this.proc = null;
    this.addr = '';
    this.token = '';
    this.runtimeDir = path.join(app.getPath('userData'), 'run');
    this.discoveryFile = path.join(this.runtimeDir, 'daemon.json');
  }

  async start() {
    const bin = daemonBinaryPath();
    if (!fs.existsSync(bin)) {
      throw new Error(`virtad binary not found at ${bin} (run \`make app\` to stage it)`);
    }

    await fsp.mkdir(this.runtimeDir, { recursive: true });
    // Drop any discovery file left by a previous run so we never read a dead daemon's address.
    await fsp.rm(this.discoveryFile, { force: true });

    // VIRTA_RUNTIME_DIR pins where the daemon writes daemon.json so we can read it deterministically;
    // VIRTA_ADDR=127.0.0.1:0 lets the OS pick a free port (the daemon writes the chosen one back).
    this.proc = spawn(bin, [], {
      env: { ...process.env, VIRTA_RUNTIME_DIR: this.runtimeDir, VIRTA_ADDR: '127.0.0.1:0' },
      stdio: 'ignore',
      windowsHide: true,
    });
    this.proc.on('error', (e) => console.error('[virtad] spawn error:', e));
    this.proc.on('exit', (code, signal) => {
      if (code !== 0 && code !== null) console.error(`[virtad] exited code=${code} signal=${signal}`);
      this.proc = null;
    });

    const deadline = Date.now() + START_TIMEOUT_MS;
    while (Date.now() < deadline) {
      if (!this.proc) throw new Error('virtad exited before becoming ready');
      const d = await readDiscovery(this.discoveryFile);
      if (d && (await probeHealth(d.addr))) {
        this.addr = d.addr;
        this.token = d.token;
        return;
      }
      await sleep(POLL_INTERVAL_MS);
    }
    throw new Error('virtad did not become ready within timeout');
  }

  discovery() {
    return this.addr && this.token ? { addr: this.addr, token: this.token } : null;
  }

  // stop terminates the child. Node's kill() maps to TerminateProcess on Windows, so unlike the
  // Wails shell's SIGTERM (a no-op there) the daemon actually dies with the app.
  stop() {
    if (this.proc && !this.proc.killed) {
      try { this.proc.kill(); } catch { /* already gone */ }
    }
    this.proc = null;
  }
}

module.exports = { Daemon };
