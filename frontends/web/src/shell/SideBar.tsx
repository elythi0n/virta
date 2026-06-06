import { useState } from 'react';
import { Badge, Button, Segmented, Select, StatusDot, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { PANEL_CATALOG, SOURCES, type ViewId } from './views';
import styles from './SideBar.module.css';

type Props = {
  view: ViewId;
  theme: string;
  openPanel: (kind: string, title: string) => void;
  setTheme: (t: string) => void;
};

const TITLES: Record<ViewId, string> = {
  panels: 'Panels',
  sources: 'Sources',
  settings: 'Settings',
};

export default function SideBar({ view, theme, openPanel, setTheme }: Props) {
  // Density is a placeholder until the feed renderer consumes it; local for now.
  const [density, setDensity] = useState('cozy');

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

        {view === 'settings' && (
          <>
            <div className={styles.field}>
              <Text as="span" variant="meta" tone="subtle" className={styles.fieldLabel}>
                Appearance
              </Text>
              <Segmented
                ariaLabel="Theme"
                value={theme}
                onValueChange={setTheme}
                options={[
                  { value: 'graphite-dark', label: 'Graphite' },
                  { value: 'light', label: 'Light' },
                ]}
              />
            </div>
            <div className={styles.field}>
              <Text as="span" variant="meta" tone="subtle" className={styles.fieldLabel}>
                Feed density
              </Text>
              <Select
                ariaLabel="Feed density"
                value={density}
                onValueChange={setDensity}
                options={[
                  { value: 'compact', label: 'Compact' },
                  { value: 'cozy', label: 'Cozy' },
                  { value: 'comfortable', label: 'Comfortable' },
                ]}
              />
            </div>
          </>
        )}
      </div>
    </aside>
  );
}
