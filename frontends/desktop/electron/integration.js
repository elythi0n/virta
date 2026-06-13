'use strict';

// Port of the former Wails shell's integration.go: reports which rung of each OS-integration
// fallback chain is active, as machine codes the web UI maps to user-facing copy. Kept identical
// to the Go version so the "Platform integration" settings panel renders unchanged.

function currentSession() {
  const t = process.env.XDG_SESSION_TYPE;
  if (t === 'wayland' || t === 'x11') return t;
  if (process.env.WAYLAND_DISPLAY) return 'wayland';
  if (process.env.DISPLAY) return 'x11';
  return '';
}

// resolveIntegration mirrors resolveIntegration(goos, session) from the Go shell.
function resolveIntegration() {
  const goos = { win32: 'windows', darwin: 'darwin', linux: 'linux' }[process.platform] || process.platform;
  const session = goos === 'linux' ? currentSession() : '';
  const wayland = goos === 'linux' && session === 'wayland';

  const hotkeys = { id: 'hotkeys', rung: 'in_app', detail: wayland ? 'wayland_restricted' : 'not_implemented' };
  const windowFeat = { id: 'window', rung: 'native' };
  if (wayland) windowFeat.detail = 'wayland_no_self_position';

  const report = {
    os: goos,
    features: [
      windowFeat,
      { id: 'theme', rung: 'native' }, // system light/dark is followed
      { id: 'quicklaunch', rung: 'in_app' }, // Ctrl+K profile switcher is always present
      hotkeys, // in-app shortcuts; native/portal not yet wired
      { id: 'notifications', rung: 'in_app' }, // in-app banner; native toast not yet wired
      { id: 'tray', rung: 'none' }, // close-to-quit; native tray not yet wired
      { id: 'sounds', rung: 'visual' }, // visual flash; audio output not yet wired
    ],
  };
  if (session) report.session = session;
  return report;
}

module.exports = { resolveIntegration };
