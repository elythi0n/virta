import { useState } from 'react';
import { Button, Popover, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { useProfiles } from '../daemon';
import AccountMenu from './AccountMenu';
import styles from './Titlebar.module.css';

type Props = {
  onOpenPalette: () => void;
};

export default function Titlebar({ onOpenPalette }: Props) {
  const { profiles, active, status: profilesStatus, activate } = useProfiles();
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <header className={styles.bar}>
      <div className={styles.left}>
        <span className={styles.brand}>Virta</span>
        <Popover
          open={menuOpen}
          onOpenChange={setMenuOpen}
          align="start"
          trigger={
            <Button variant="ghost" size="sm" className={styles.profileBtn} disabled={profilesStatus === 'offline'}>
              {active?.name ?? 'Profile'}
              <Icon name="chevron-down" size={14} />
            </Button>
          }
        >
          <div className={styles.menu} role="menu" aria-label="Profiles">
            {profiles.length === 0 ? (
              <Text variant="meta" tone="subtle" as="p" className={styles.menuEmpty}>
                No profiles
              </Text>
            ) : (
              profiles.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  role="menuitemradio"
                  aria-checked={p.active}
                  className={styles.menuItem}
                  onClick={() => {
                    setMenuOpen(false);
                    if (!p.active) void activate(p.id);
                  }}
                >
                  <span className={styles.check}>{p.active ? '✓' : ''}</span>
                  <span className={styles.menuName}>{p.name}</span>
                  {p.default && (
                    <Text variant="meta" tone="subtle">
                      default
                    </Text>
                  )}
                </button>
              ))
            )}
          </div>
        </Popover>
      </div>

      <div className={styles.center}>
        <button type="button" className={styles.search} onClick={onOpenPalette} aria-label="Search commands">
          <span className={styles.searchLeft}>
            <Icon name="search" size={14} />
            <span className={styles.searchText}>Search commands…</span>
          </span>
          <kbd className={styles.kbd}>⌃⇧P</kbd>
        </button>
      </div>

      {/* Right: account menu (shown only in hosted mode; invisible in local/desktop mode). */}
      <div className={styles.right}>
        <AccountMenu />
      </div>
    </header>
  );
}
