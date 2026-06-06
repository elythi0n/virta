import { Badge, Button, StatusDot, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { PANEL_CATALOG, SOURCES, type ViewId } from './views';
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

        {view === 'sources' && (
          <>
            <ul className={styles.rows}>
              {SOURCES.map((s) => (
                <li key={s.id}>
                  <div className={styles.sourceRow}>
                    <span className={styles.rail} style={{ background: s.accent }} />
                    <StatusDot status="offline" label={`${s.label}: not connected`} />
                    <Text variant="ui" tone="muted" className={styles.label}>
                      {s.label}
                    </Text>
                    <Badge tone="neutral">Not connected</Badge>
                  </div>
                </li>
              ))}
            </ul>
            <Text as="p" variant="meta" tone="subtle" className={styles.note}>
              Connecting accounts wires to the daemon in a later step.
            </Text>
          </>
        )}
      </div>
    </aside>
  );
}
