import { useState } from 'react';
import { Popover } from '@virta/ui-kit';
import type { Density } from '@virta/feed-core';
import Icon from '../Icon';
import styles from './DensityControl.module.css';

// Five text-size steps, smallest to largest, mapped to the chat type-token scale.
export const DENSITIES: { value: Density; label: string }[] = [
  { value: 'tiny', label: 'Extra small' },
  { value: 'compact', label: 'Small' },
  { value: 'cozy', label: 'Default' },
  { value: 'comfortable', label: 'Large' },
  { value: 'large', label: 'Extra large' },
];

// Per-feed text-size control: a compact popover so each chat tab can pick its own zoom without
// crowding the toolbar. Each option previews its own size.
export default function DensityControl({ value, onChange }: { value: Density; onChange: (d: Density) => void }) {
  const [open, setOpen] = useState(false);
  const pick = (d: Density) => {
    onChange(d);
    setOpen(false);
  };
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      align="end"
      trigger={
        <button type="button" className={styles.trigger} aria-label="Chat text size">
          <Icon name="text-size" size={15} />
          <Icon name="chevron-down" size={12} />
        </button>
      }
    >
      <div className={styles.menu} role="menu" aria-label="Chat text size">
        {DENSITIES.map((d) => (
          <button
            key={d.value}
            type="button"
            role="menuitemradio"
            aria-checked={d.value === value}
            className={`${styles.item} ${d.value === value ? styles.active : ''}`}
            onClick={() => pick(d.value)}
          >
            <span className={styles.sample} data-size={d.value}>
              Aa
            </span>
            <span className={styles.itemLabel}>{d.label}</span>
            {d.value === value && (
              <span className={styles.check} aria-hidden>
                ✓
              </span>
            )}
          </button>
        ))}
      </div>
    </Popover>
  );
}
