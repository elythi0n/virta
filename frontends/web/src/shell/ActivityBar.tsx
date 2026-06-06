import Icon from '../Icon';
import { PRIMARY_VIEWS, FOOTER_VIEWS, type ViewDef, type ViewId } from './views';
import styles from './ActivityBar.module.css';

type Props = {
  activeView: ViewId;
  sidebarOpen: boolean;
  onSelect: (v: ViewId) => void;
};

export default function ActivityBar({ activeView, sidebarOpen, onSelect }: Props) {
  const isActive = (id: ViewId) => sidebarOpen && activeView === id;

  const item = (v: ViewDef) => {
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
      <div className={styles.group}>{PRIMARY_VIEWS.map(item)}</div>
      <div className={styles.group}>{FOOTER_VIEWS.map(item)}</div>
    </nav>
  );
}
