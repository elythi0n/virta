import { Button, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { PANEL_CATALOG, type ViewId } from './views';
import Sources from './Sources';
import styles from './SideBar.module.css';

type Props = {
  view: ViewId;
  openPanel: (kind: string, title: string) => void;
};

const TITLES: Record<ViewId, string> = {
  panels: 'Panels',
  sources: 'Sources',
};

export default function SideBar({ view, openPanel }: Props) {
  return (
    <aside className={styles.side} aria-label={TITLES[view]}>
      <Text as="header" variant="meta" tone="subtle" className={styles.head}>
        {TITLES[view]}
      </Text>
      <div className={styles.body}>
        {view === 'panels' && (
          <ul className={styles.rows}>
            {PANEL_CATALOG.map((p) => (
              <li key={p.kind}>
                <Button variant="ghost" size="sm" className={styles.rowBtn} onClick={() => openPanel(p.kind, p.title)}>
                  <span className={styles.glyph}>
                    <Icon name={p.icon} size={16} />
                  </span>
                  <Text variant="ui" tone="inherit" className={styles.rowLabel}>
                    {p.title}
                  </Text>
                </Button>
              </li>
            ))}
          </ul>
        )}

        {view === 'sources' && <Sources />}
      </div>
    </aside>
  );
}
