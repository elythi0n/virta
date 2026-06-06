import FeedPanel from './FeedPanel';
import styles from './Panel.module.css';

// Routes a dock panel kind to its content. Feed-like kinds render the live feed; the rest are
// placeholders until their panels are built.
export default function Panel({ kind }: { kind: string }) {
  if (kind === 'feed' || kind === 'x-chat') {
    return <FeedPanel />;
  }
  return (
    <div className={styles.placeholder}>
      <span className={styles.label}>{kind}</span>
      <span className={styles.hint}>panel content lands in a later step</span>
    </div>
  );
}
