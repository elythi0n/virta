import { useChannels } from '../daemon';
import styles from './ConnectionAlert.module.css';

// A full-width strip shown only when the daemon can't be reached, so the chrome stays quiet when
// all is well (no steady "Live" readout). useChannels polls, so this appears and clears on its own.
export default function ConnectionAlert() {
  const { status } = useChannels();
  if (status !== 'offline') return null;
  return (
    <div className={styles.alert} role="status" aria-live="polite">
      <span className={styles.dot} aria-hidden />
      Not connected to the daemon. Retrying…
    </div>
  );
}
