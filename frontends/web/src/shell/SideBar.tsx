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

const THEMES = [
  { id: 'graphite-dark', label: 'Graphite' },
  { id: 'light', label: 'Light' },
];

export default function SideBar({ view, theme, openPanel, setTheme }: Props) {
  return (
    <aside className={styles.side} aria-label={TITLES[view]}>
      <header className={styles.head}>{TITLES[view]}</header>
      <div className={styles.body}>
        {view === 'panels' && (
          <ul className={styles.rows}>
            {PANEL_CATALOG.map((p) => (
              <li key={p.kind}>
                <button className={styles.row} onClick={() => openPanel(p.kind, p.title)}>
                  <span className={styles.glyph}>
                    <Icon name={p.icon} size={16} />
                  </span>
                  <span className={styles.label}>{p.title}</span>
                </button>
              </li>
            ))}
          </ul>
        )}

        {view === 'sources' && (
          <>
            <ul className={styles.rows}>
              {SOURCES.map((s) => (
                <li key={s.id}>
                  <div className={`${styles.row} ${styles.static}`}>
                    <span className={styles.rail} style={{ background: s.accent }} />
                    <span className={styles.label}>{s.label}</span>
                    <span className={styles.state}>Not connected</span>
                  </div>
                </li>
              ))}
            </ul>
            <p className={styles.note}>Connecting accounts wires to the daemon in a later step.</p>
          </>
        )}

        {view === 'settings' && (
          <div className={styles.field}>
            <span className={styles.fieldLabel}>Appearance</span>
            <div className={styles.segmented} role="group" aria-label="Theme">
              {THEMES.map((t) => (
                <button
                  key={t.id}
                  className={`${styles.seg} ${theme === t.id ? styles.on : ''}`}
                  aria-pressed={theme === t.id}
                  onClick={() => setTheme(t.id)}
                >
                  {t.label}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </aside>
  );
}
