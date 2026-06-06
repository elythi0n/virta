import * as RC from '@radix-ui/react-context-menu';
import type { ReactNode } from 'react';
import styles from './ContextMenu.module.css';

export type ContextMenuEntry =
  | { kind: 'item'; label: string; onSelect: () => void; disabled?: boolean; danger?: boolean }
  | { kind: 'separator' };

type ContextMenuProps = {
  /** The element a right-click opens the menu on. */
  trigger: ReactNode;
  items: ContextMenuEntry[];
};

// Token-styled right-click menu on Radix (cursor positioning, keyboard nav, dismissal, ARIA).
export default function ContextMenu({ trigger, items }: ContextMenuProps) {
  return (
    <RC.Root>
      <RC.Trigger asChild>{trigger}</RC.Trigger>
      <RC.Portal>
        <RC.Content className={styles.content}>
          {items.map((entry, i) =>
            entry.kind === 'separator' ? (
              <RC.Separator key={i} className={styles.separator} />
            ) : (
              <RC.Item
                key={i}
                className={`${styles.item} ${entry.danger ? styles.danger : ''}`}
                disabled={entry.disabled}
                onSelect={entry.onSelect}
              >
                {entry.label}
              </RC.Item>
            ),
          )}
        </RC.Content>
      </RC.Portal>
    </RC.Root>
  );
}
