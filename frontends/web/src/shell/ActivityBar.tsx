import Icon from '../Icon';
import { PRIMARY_VIEWS, type ViewDef, type ViewId } from './views';
import styles from './ActivityBar.module.css';

type Props = {
  activeView: ViewId;
  sidebarOpen: boolean;
  onSelect: (v: ViewId) => void;
  onOpenSettings: () => void;
};

export default function ActivityBar({ activeView, sidebarOpen, onSelect, onOpenSettings }: Props) {
  const isActive = (id: ViewId) => sidebarOpen && activeView === id;

  const viewItem = (v: ViewDef) => {
    const active = isActive(v.id);
    return (
      <button
        key={v.id}
        className={`${styles.item} ${active ? styles.active : ''}`}
        title={v.label}
        aria-label={v.label}
        aria-pressed={active}
        onClick={() => onSelect(v.id)}
      >
        <Icon name={v.icon} />
      </button>
    );
  };

  return (
    <nav className={styles.bar} aria-label="Primary">
      <div className={styles.group}>{PRIMARY_VIEWS.map(viewItem)}</div>
      <div className={styles.group}>
        {/* Settings is an action, not a view: it opens a dock panel rather than the side bar. */}
        <button className={styles.item} title="Settings" aria-label="Settings" onClick={onOpenSettings}>
          <Icon name="settings" />
        </button>
      </div>
    </nav>
  );
}
