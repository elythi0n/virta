import type { ReactNode } from 'react';
import styles from './Tabs.module.css';

export type TabItem = { value: string; label: ReactNode; icon?: ReactNode };

type TabsProps = {
  items: TabItem[];
  value: string;
  onValueChange: (value: string) => void;
  ariaLabel?: string;
  /** Visual treatment: an underline bar (default) or a contained pill group. */
  variant?: 'underline' | 'pill';
};

// A controlled tab strip. Content is rendered by the caller based on `value`; this is purely the
// selector so it composes with any layout.
export default function Tabs({ items, value, onValueChange, ariaLabel, variant = 'underline' }: TabsProps) {
  return (
    <div className={`${styles.tabs} ${styles[variant]}`} role="tablist" aria-label={ariaLabel}>
      {items.map((it) => (
        <button
          key={it.value}
          type="button"
          role="tab"
          aria-selected={value === it.value}
          className={`${styles.tab} ${value === it.value ? styles.on : ''}`}
          onClick={() => onValueChange(it.value)}
        >
          {it.icon && <span className={styles.icon}>{it.icon}</span>}
          {it.label}
        </button>
      ))}
    </div>
  );
}
