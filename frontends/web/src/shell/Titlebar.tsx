import { useRef, useState } from 'react';
import { Button, Popover, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { useProfiles } from '../daemon';
import AccountMenu from './AccountMenu';
import { useIsDesktop } from './useIsDesktop';
import styles from './Titlebar.module.css';


type Props = {
  onOpenPalette: () => void;
};

export default function Titlebar({ onOpenPalette }: Props) {
  const { profiles, active, status: profilesStatus, activate, create, remove } = useProfiles();
  const [menuOpen, setMenuOpen] = useState(false);
  const isDesktop = useIsDesktop();
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const [createError, setCreateError] = useState('');

  const handleCreate = async () => {
    const name = newName.trim();
    if (!name) return;
    setCreating(true);
    setCreateError('');
    try {
      await create(name);
      setNewName('');
    } catch (e: unknown) {
      setCreateError(e instanceof Error ? e.message : 'Failed to create workspace');
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setDeletingId(id);
    try {
      await remove(id);
    } finally {
      setDeletingId(null);
    }
  };

  const handleOpenMenu = (open: boolean) => {
    setMenuOpen(open);
    if (open) setNewName('');
  };

  return (
    <header className={styles.bar}>
      <div className={styles.left}>
        <span className={styles.brand}>Virta</span>
        <span className={styles.betaBadge} aria-label="Beta">Beta</span>
        <Popover
          open={menuOpen}
          onOpenChange={handleOpenMenu}
          align="start"
          trigger={
            <Button variant="ghost" size="sm" className={styles.profileBtn} disabled={profilesStatus === 'offline'}>
              {active?.name ?? 'Workspace'}
              <Icon name="chevron-down" size={14} />
            </Button>
          }
        >
          <div className={styles.menu} role="menu" aria-label="Workspaces">
            <div className={styles.menuHeader}>
              <Text variant="meta" tone="subtle">Workspaces</Text>
            </div>

            {profiles.length === 0 ? (
              <Text variant="meta" tone="subtle" as="p" className={styles.menuEmpty}>
                No workspaces
              </Text>
            ) : (
              <div className={styles.menuList}>
                {profiles.map((p) => (
                  <div key={p.id} className={styles.menuRow}>
                    <button
                      type="button"
                      role="menuitemradio"
                      aria-checked={p.active}
                      className={styles.menuItem}
                      data-active={p.active}
                      onClick={() => {
                        if (!p.active) void activate(p.id);
                        setMenuOpen(false);
                      }}
                    >
                      <span className={styles.activeIndicator} aria-hidden>
                        {p.active && <Icon name="check" size={12} />}
                      </span>
                      <span className={styles.menuName}>{p.name}</span>
                      {p.default && <span className={styles.defaultBadge}>default</span>}
                    </button>
                    {!p.default && !p.active && (
                      <button
                        type="button"
                        className={styles.deleteBtn}
                        aria-label={`Delete ${p.name}`}
                        disabled={deletingId === p.id}
                        onClick={(e) => void handleDelete(p.id, e)}
                      >
                        <Icon name="x" size={12} />
                      </button>
                    )}
                  </div>
                ))}
              </div>
            )}

            <div className={styles.menuDivider} />

            <div className={styles.newProfile}>
              <input
                ref={inputRef}
                className={styles.newProfileInput}
                type="text"
                placeholder="New workspace name"
                value={newName}
                onChange={(e) => { setNewName(e.currentTarget.value); setCreateError(''); }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') void handleCreate();
                  if (e.key === 'Escape') { setNewName(''); setMenuOpen(false); }
                }}
                disabled={creating}
                maxLength={64}
              />
              <button
                type="button"
                className={styles.newProfileBtn}
                disabled={!newName.trim() || creating}
                onClick={() => void handleCreate()}
                aria-label="Create workspace"
              >
                <Icon name="plus" size={13} />
              </button>
            </div>
            {createError && (
              <div className={styles.createError}>{createError}</div>
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

      <div className={styles.right}>
        <AccountMenu />
        {isDesktop && (
          <div className={styles.winControls} aria-label="Window controls">
            <button
              type="button"
              className={styles.winBtn}
              aria-label="Minimise window"
              onClick={() => void window.go?.main?.App?.WindowMinimise?.()}
            >
              <Icon name="win-minimise" size={12} />
            </button>
            <button
              type="button"
              className={styles.winBtn}
              aria-label="Maximise / restore window"
              onClick={() => void window.go?.main?.App?.WindowToggleMaximise?.()}
            >
              <Icon name="win-maximise" size={12} />
            </button>
            <button
              type="button"
              className={`${styles.winBtn} ${styles.winClose}`}
              aria-label="Close window"
              onClick={() => void window.go?.main?.App?.WindowClose?.()}
            >
              <Icon name="win-close" size={12} />
            </button>
          </div>
        )}
      </div>
    </header>
  );
}
