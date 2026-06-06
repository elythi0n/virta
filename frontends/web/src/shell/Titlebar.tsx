import { useState } from 'react';
import { Button, Popover, StatusDot, Text, type DotStatus } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels, useProfiles } from '../daemon';
import styles from './Titlebar.module.css';

type Props = {
  onOpenPalette: () => void;
};

export default function Titlebar({ onOpenPalette }: Props) {
  const { profiles, active, status: profilesStatus, activate } = useProfiles();
  const { channels, status: channelsStatus } = useChannels();
  const [menuOpen, setMenuOpen] = useState(false);

  const connected = channels.some((c) => c.state === 'connected');
  const dot: DotStatus = channelsStatus === 'offline' ? 'offline' : connected ? 'live' : 'idle';
  const liveLabel =
    channelsStatus === 'offline'
      ? 'Disconnected'
      : connected
        ? `${channels.filter((c) => c.state === 'connected').length} live`
        : 'Connected';

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

      <div className={styles.right}>
        <span className={styles.live}>
          <StatusDot status={dot} label={liveLabel} />
          <Text variant="meta" tone="subtle">
            {liveLabel}
          </Text>
        </span>
      </div>
    </header>
  );
}
