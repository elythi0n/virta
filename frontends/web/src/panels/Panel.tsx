import { panelByKind } from './registry';
import styles from './Panel.module.css';

// Routes a dock panel kind to its content by looking it up in the contribution registry — no
// hardcoded per-kind switch, so our panels and (later) plugin panels open the same way. An unknown
// kind (e.g. a removed plugin) renders a graceful placeholder rather than a blank pane.
export default function Panel({ kind, channels, panelId }: { kind: string; channels?: string[]; panelId?: string }) {
  const contribution = panelByKind(kind);
  if (contribution) return <>{contribution.render({ channels, panelId })}</>;
  return (
    <div className={styles.placeholder}>
      <span className={styles.label}>{kind}</span>
      <span className={styles.hint}>This panel kind is not registered. It may be from a plugin that is no longer installed.</span>
    </div>
  );
}
