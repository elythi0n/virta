import { Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import AccountItem from './AccountItem';
import { PRIMARY_VIEWS, type ViewDef, type ViewId } from './views';
import styles from './ActivityBar.module.css';

type Props = {
  activeView: ViewId;
  sidebarOpen: boolean;
  onSelect: (v: ViewId) => void;
  onOpenSettings: () => void;
  onOpenPlugins: () => void;
};

export default function ActivityBar({ activeView, sidebarOpen, onSelect, onOpenSettings, onOpenPlugins }: Props) {
  const isActive = (id: ViewId) => sidebarOpen && activeView === id;

  const viewItem = (v: ViewDef) => {
    const active = isActive(v.id);
    return (
      <Tooltip key={v.id} content={v.label} side="right">
        <button
          className={`${styles.item} ${active ? styles.active : ''}`}
          aria-label={v.label}
          aria-pressed={active}
          onClick={() => onSelect(v.id)}
        >
          <Icon name={v.icon} size={23} />
        </button>
      </Tooltip>
    );
  };

  return (
    <nav className={styles.bar} aria-label="Primary">
      <div className={styles.group}>
        {PRIMARY_VIEWS.map(viewItem)}
        {/* Plugins sits with the views but is an action: it opens a dock panel, not the side bar. */}
        <Tooltip content="Plugins" side="right">
          <button className={styles.item} aria-label="Plugins" onClick={onOpenPlugins}>
            <Icon name="plugins" size={23} />
          </button>
        </Tooltip>
      </div>
      <div className={styles.group}>
        {/* Account — only rendered in hosted mode (AccountItem returns null otherwise). */}
        <AccountItem />
        <div className={styles.divider} />
        {/* Settings is an action too: it opens the Settings panel rather than the side bar. */}
        <Tooltip content="Settings" side="right">
          <button className={styles.item} aria-label="Settings" onClick={onOpenSettings}>
            <Icon name="settings" size={23} />
          </button>
        </Tooltip>
      </div>
    </nav>
  );
}
