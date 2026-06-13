import { useSyncExternalStore } from 'react';
import { panelByKind, subscribePanelCatalog, panelCatalogVersion } from './registry';
import styles from './Panel.module.css';

// Routes a dock panel kind to its content by looking it up in the contribution registry — no
// hardcoded per-kind switch, so our panels and (later) plugin panels open the same way. An unknown
// kind (e.g. a removed plugin) renders a graceful placeholder rather than a blank pane.
//
// useSyncExternalStore subscribes to catalogVersion so this component re-renders when
// syncPluginPanels() registers a new kind after mount (async, may arrive after first render).
export default function Panel({ kind, channels, panelId }: { kind: string; channels?: string[]; panelId?: string }) {
  useSyncExternalStore(subscribePanelCatalog, panelCatalogVersion);
  const contribution = panelByKind(kind);
  if (contribution) return <>{contribution.render({ channels, panelId })}</>;
  return (
    <div className={styles.placeholder}>
      <span className={styles.label}>{kind}</span>
      <span className={styles.hint}>This panel kind is not registered. It may be from a plugin that is no longer installed.</span>
    </div>
  );
}
