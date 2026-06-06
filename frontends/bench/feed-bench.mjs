// Feed performance benchmark (ADR-010 gate): drive the real app at 200 msg/s in a headless
// browser and measure main-thread jank. Uses playwright-core against the system Chromium, so it
// needs no browser download. Run deliberately (like the live tests), not in the offline `make ci`.
//
//   npm run bench:feed        (from frontends/)
//
// Builds web, serves the production build, opens the Chat feed, sets the rate to 200/s, and
// records frame timing + long-task blocking over a fixed window. Exits non-zero if it busts the
// budget, so it can gate a PR where a browser is available.
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { chromium } from 'playwright-core';

const PORT = 4173;
const URL = `http://localhost:${PORT}/`;
const WINDOW_SECONDS = 8;
const RATE_LABEL = '200/s';

// Budget: generous enough not to flake on a loaded CI box, tight enough to catch a real
// regression (a feed that drops to a slideshow under load).
const BUDGET = { minAvgFps: 30, maxP95FrameMs: 40, maxBlockingRatio: 0.5 };

function findChromium() {
  for (const p of ['/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/google-chrome-stable', '/usr/bin/google-chrome']) {
    if (existsSync(p)) return p;
  }
  throw new Error('no system Chromium found; install chromium or google-chrome');
}

function sh(cmd, args, opts = {}) {
  return new Promise((resolve, reject) => {
    const p = spawn(cmd, args, { stdio: 'inherit', ...opts });
    p.on('exit', (code) => (code === 0 ? resolve() : reject(new Error(`${cmd} ${args.join(' ')} exited ${code}`))));
  });
}

async function waitForServer(url, timeoutMs = 30000) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const r = await fetch(url);
      if (r.ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error('preview server did not become ready');
}

const measure = async (seconds) => {
  const longTasks = [];
  const observer = new PerformanceObserver((list) => {
    for (const e of list.getEntries()) longTasks.push(e.duration);
  });
  try {
    observer.observe({ entryTypes: ['longtask'] });
  } catch {
    // longtask unsupported; blocking metrics will read zero
  }

  const intervals = [];
  let last = performance.now();
  let running = true;
  const tick = (now) => {
    intervals.push(now - last);
    last = now;
    if (running) requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);

  await new Promise((r) => setTimeout(r, seconds * 1000));
  running = false;
  observer.disconnect();

  const samples = intervals.slice(1).sort((a, b) => a - b); // drop the first (warm) interval
  const sum = samples.reduce((a, b) => a + b, 0);
  const quantile = (q) => (samples.length ? samples[Math.min(samples.length - 1, Math.floor(q * samples.length))] : 0);
  const blockingMs = longTasks.reduce((a, d) => a + Math.max(0, d - 50), 0);

  return {
    frames: samples.length,
    avgFps: samples.length ? 1000 / (sum / samples.length) : 0,
    p50FrameMs: quantile(0.5),
    p95FrameMs: quantile(0.95),
    maxFrameMs: samples.length ? samples[samples.length - 1] : 0,
    longTasks: longTasks.length,
    totalBlockingMs: Math.round(blockingMs),
    windowMs: Math.round(sum),
  };
};

async function main() {
  const executablePath = findChromium();
  console.log('> building web (production)…');
  await sh('npm', ['--prefix', 'web', 'run', 'build']);

  console.log('> serving build…');
  const preview = spawn('npm', ['--prefix', 'web', 'run', 'preview', '--', '--port', String(PORT), '--strictPort'], {
    stdio: 'ignore',
  });

  let metrics;
  try {
    await waitForServer(URL);
    const browser = await chromium.launch({ executablePath, headless: true });
    try {
      const page = await browser.newPage({ viewport: { width: 1366, height: 900 } });
      await page.goto(URL, { waitUntil: 'networkidle' });
      await page.getByText('Chat', { exact: true }).first().click();
      await page.getByRole('button', { name: RATE_LABEL }).click();
      await page.waitForTimeout(1200); // let the stream reach steady state
      console.log(`> measuring ${WINDOW_SECONDS}s at ${RATE_LABEL}…`);
      metrics = await page.evaluate(measure, WINDOW_SECONDS);
    } finally {
      await browser.close();
    }
  } finally {
    preview.kill('SIGTERM');
  }

  const blockingRatio = metrics.totalBlockingMs / metrics.windowMs;
  console.log('\n  feed @ 200 msg/s');
  console.log(`  frames           ${metrics.frames}`);
  console.log(`  avg fps          ${metrics.avgFps.toFixed(1)}`);
  console.log(`  frame p50 / p95  ${metrics.p50FrameMs.toFixed(1)}ms / ${metrics.p95FrameMs.toFixed(1)}ms`);
  console.log(`  frame max        ${metrics.maxFrameMs.toFixed(1)}ms`);
  console.log(`  long tasks       ${metrics.longTasks}`);
  console.log(`  blocking         ${metrics.totalBlockingMs}ms of ${metrics.windowMs}ms (${(blockingRatio * 100).toFixed(0)}%)\n`);

  const failures = [];
  if (metrics.avgFps < BUDGET.minAvgFps) failures.push(`avg fps ${metrics.avgFps.toFixed(1)} < ${BUDGET.minAvgFps}`);
  if (metrics.p95FrameMs > BUDGET.maxP95FrameMs) failures.push(`p95 frame ${metrics.p95FrameMs.toFixed(1)}ms > ${BUDGET.maxP95FrameMs}ms`);
  if (blockingRatio > BUDGET.maxBlockingRatio) failures.push(`blocking ${(blockingRatio * 100).toFixed(0)}% > ${BUDGET.maxBlockingRatio * 100}%`);

  if (failures.length) {
    console.error('FAIL: ' + failures.join('; '));
    process.exit(1);
  }
  console.log('PASS: feed stays smooth at 200 msg/s');
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
