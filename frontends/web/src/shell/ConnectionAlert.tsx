import { useChannels } from '../daemon';
import styles from './ConnectionAlert.module.css';

// A full-width strip shown only when the daemon is unreachable. Quiet when all is well.
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
