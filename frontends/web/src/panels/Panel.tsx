import FeedPanel from './FeedPanel';
import MentionInbox from './MentionInbox';
import StreamPane from './StreamPane';
import styles from './Panel.module.css';

// Routes a dock panel kind to its content. Feed-like kinds render the live feed (optionally scoped
// to a channel set); 'watch' embeds a stream's player; the rest are placeholders until built.
export default function Panel({ kind, channels, panelId }: { kind: string; channels?: string[]; panelId?: string }) {
  if (kind === 'feed' || kind === 'x-chat') {
    return <FeedPanel channels={channels} panelId={panelId} />;
  }
  if (kind === 'watch') {
    return <StreamPane channel={channels?.[0]} />;
  }
  if (kind === 'mentions') {
    return <MentionInbox />;
  }
  return (
    <div className={styles.placeholder}>
      <span className={styles.label}>{kind}</span>
      <span className={styles.hint}>panel content lands in a later step</span>
    </div>
  );
}
