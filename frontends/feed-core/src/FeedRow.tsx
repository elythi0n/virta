import { memo } from 'react';
import type { FeedMessage } from './types';
import styles from './FeedRow.module.css';

// The single most-rendered element. Bespoke (not a library primitive) and memoized so streaming a
// new message never re-renders the rows above it. A left rail carries platform identity.
function FeedRow({ message }: { message: FeedMessage }) {
  return (
    <div className={`${styles.row} ${styles[message.platform]}`}>
      <span className={styles.ts}>{message.ts}</span>
      <span className={styles.author}>{message.author}</span>
      <span className={styles.body}>{message.body}</span>
    </div>
  );
}

export default memo(FeedRow);
