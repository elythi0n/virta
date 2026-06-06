import * as RC from '@radix-ui/react-context-menu';
import type { ReactNode } from 'react';
import styles from './ContextMenu.module.css';

export type ContextMenuEntry =
  | { kind: 'item'; label: string; onSelect: () => void; disabled?: boolean; danger?: boolean }
  | { kind: 'submenu'; label: string; items: ContextMenuEntry[]; disabled?: boolean }
  | { kind: 'separator' };

type ContextMenuProps = {
  /** The element a right-click opens the menu on. */
  trigger: ReactNode;
  items: ContextMenuEntry[];
};

function renderEntries(items: ContextMenuEntry[]): ReactNode {
  return items.map((entry, i) => {
    if (entry.kind === 'separator') {
      return <RC.Separator key={i} className={styles.separator} />;
    }
    if (entry.kind === 'submenu') {
      return (
        <RC.Sub key={i}>
          <RC.SubTrigger className={styles.item} disabled={entry.disabled}>
            {entry.label}
            <span className={styles.chevron} aria-hidden>
              ›
            </span>
          </RC.SubTrigger>
          <RC.Portal>
            <RC.SubContent className={styles.content} sideOffset={2} alignOffset={-4}>
              {renderEntries(entry.items)}
            </RC.SubContent>
          </RC.Portal>
        </RC.Sub>
      );
    }
    return (
      <RC.Item
        key={i}
        className={`${styles.item} ${entry.danger ? styles.danger : ''}`}
        disabled={entry.disabled}
        onSelect={entry.onSelect}
      >
        {entry.label}
      </RC.Item>
    );
  });
}

// Token-styled right-click menu on Radix (cursor positioning, keyboard nav, dismissal, ARIA), with
// optional nested submenus.
export default function ContextMenu({ trigger, items }: ContextMenuProps) {
  return (
    <RC.Root>
      <RC.Trigger asChild>{trigger}</RC.Trigger>
      <RC.Portal>
        <RC.Content className={styles.content}>{renderEntries(items)}</RC.Content>
      </RC.Portal>
    </RC.Root>
  );
}
