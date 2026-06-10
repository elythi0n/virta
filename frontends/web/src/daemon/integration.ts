import { useEffect, useState } from 'react';

// Which rung of an OS-integration fallback chain is active, as a machine code the UI maps
// to user copy. Served by the desktop shell at /__integration; the browser build has no shell.
export interface IntegrationFeature {
  id: string;
  rung: string;
  detail?: string;
}

export interface IntegrationReport {
  os: string;
  session?: string;
  features: IntegrationFeature[];
}

// Running in a plain browser against the daemon (no native shell): every feature is on its in-app /
// browser rung. The system theme is still followed (matchMedia), and the Ctrl+K switcher is present.
const WEB_FALLBACK: IntegrationReport = {
  os: 'web',
  features: [
    { id: 'window', rung: 'browser' },
    { id: 'theme', rung: 'native' },
    { id: 'quicklaunch', rung: 'in_app' },
    { id: 'hotkeys', rung: 'in_app', detail: 'browser' },
    { id: 'notifications', rung: 'in_app' },
    { id: 'tray', rung: 'none' },
    { id: 'sounds', rung: 'visual' },
  ],
};

// getIntegration reads the shell's report at the same origin (like /__discovery); anything else (a
// browser tab served by the daemon, a dev server) has no such endpoint and falls back to web rungs.
export function getIntegration(): Promise<IntegrationReport> {
  return fetch('/__integration')
    .then((r) => (r.ok ? (r.json() as Promise<IntegrationReport>) : WEB_FALLBACK))
    .catch(() => WEB_FALLBACK);
}

export function useIntegration(): IntegrationReport {
  const [report, setReport] = useState<IntegrationReport>(WEB_FALLBACK);
  useEffect(() => {
    let cancelled = false;
    getIntegration().then((r) => !cancelled && setReport(r));
    return () => {
      cancelled = true;
    };
  }, []);
  return report;
}
