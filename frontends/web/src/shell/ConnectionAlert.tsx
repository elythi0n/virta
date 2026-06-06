import { useChannels, useIntegration } from '../daemon';
import styles from './ConnectionAlert.module.css';

// A full-width strip shown only when something warrants user attention: the daemon is unreachable,
// or the app is running in a browser (no native desktop shell, so tray/hotkeys/notifications are
// in-app fallbacks). Quiet when all is well — no steady "Live" readout.
export default function ConnectionAlert() {
  const { status } = useChannels();
  const integration = useIntegration();
  const isBrowser = integration.os === 'web';

  if (status !== 'offline' && !isBrowser) return null;

  if (isBrowser && status !== 'offline') {
    return (
      <div className={styles.alert} role="status" aria-live="polite">
        <span className={styles.dot} aria-hidden style={{ background: 'var(--virta-text-2)' }} />
        Running in browser — tray, global hotkeys, and native notifications are in-app fallbacks.
        Full feature parity is available in the desktop app.
      </div>
    );
  }
  return (
    <div className={styles.alert} role="status" aria-live="polite">
      <span className={styles.dot} aria-hidden />
      {status === 'offline' ? 'Not connected to the daemon. Retrying…' : null}
    </div>
  );
}
