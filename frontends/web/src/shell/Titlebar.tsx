import { useState } from 'react';
import { Button, Popover, StatusDot, Text, Tooltip, type DotStatus } from '@virta/ui-kit';
import type { Density } from '@virta/feed-core';
import Icon from '../Icon';
import { useChannels, useProfiles } from '../daemon';
import { useDensity } from '../density';
import styles from './Titlebar.module.css';

type Props = {
  onOpenPalette: () => void;
};

const DENSITIES: Density[] = ['compact', 'cozy', 'comfortable'];

export default function Titlebar({ onOpenPalette }: Props) {
  const { profiles, active, status: profilesStatus, activate } = useProfiles();
  const { channels, status: channelsStatus } = useChannels();
  const { density, setDensity } = useDensity();
  const [menuOpen, setMenuOpen] = useState(false);

  const densityIndex = DENSITIES.indexOf(density);
  const stepDensity = (delta: number) => {
    const next = DENSITIES[densityIndex + delta];
    if (next) setDensity(next);
  };

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
        <span className={styles.density} role="group" aria-label="Chat text size">
          <Tooltip content="Smaller chat text" side="bottom">
            <button
              type="button"
              className={styles.zoom}
              aria-label="Smaller chat text"
              disabled={densityIndex <= 0}
              onClick={() => stepDensity(-1)}
            >
              A&minus;
            </button>
          </Tooltip>
          <Tooltip content="Larger chat text" side="bottom">
            <button
              type="button"
              className={styles.zoom}
              aria-label="Larger chat text"
              disabled={densityIndex >= DENSITIES.length - 1}
              onClick={() => stepDensity(1)}
            >
              A+
            </button>
          </Tooltip>
        </span>
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
