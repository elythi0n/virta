import styles from './Panel.module.css';

type Row = { id: number; ts: string; plat: 'twitch' | 'kick'; author: string; body: string };

// A long synthetic feed to eyeball scroll smoothness and verify the body keeps rendering
// through a drag/resize. Real virtualization and live data arrive with the feed renderer.
const SAMPLE = [
  'gg that was clean',
  'no shot he hit that',
  'first time catching the stream live',
  'what headset is that',
  'chat is going feral lol',
  'W player',
  'the pacing on this run is unreal',
];

const ROWS: Row[] = Array.from({ length: 600 }, (_, i) => ({
  id: i,
  ts: `${String(12 + Math.floor(i / 60)).padStart(2, '0')}:${String(i % 60).padStart(2, '0')}`,
  plat: i % 3 === 0 ? 'kick' : 'twitch',
  author: `viewer_${(i * 7) % 240}`,
  body: SAMPLE[i % SAMPLE.length],
}));

export default function Panel({ kind }: { kind: string }) {
  const isFeed = kind === 'feed' || kind === 'x-chat';

  if (isFeed) {
    return (
      <div className={styles.feed} role="log" aria-label="Unified feed">
        {ROWS.map((row) => (
          <div className={styles.row} key={row.id}>
            <span className={styles.ts}>{row.ts}</span>
            <span className={`${styles.dot} ${row.plat === 'kick' ? styles.kick : ''}`} />
            <span className={styles.author}>{row.author}</span>
            <span className={styles.body}>{row.body}</span>
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className={styles.placeholder}>
      <span className={styles.label}>{kind}</span>
      <span className={styles.hint}>panel content lands in a later milestone</span>
    </div>
  );
}
