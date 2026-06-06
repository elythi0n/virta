import { Dialog, formatShortcut, type CommandAction } from '@virta/ui-kit';
import styles from './ShortcutHelp.module.css';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  actions: CommandAction[];
};

// Lists every action that has a keyboard shortcut, straight from the action registry.
export default function ShortcutHelp({ open, onOpenChange, actions }: Props) {
  const bound = actions.filter((a) => a.shortcut);
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Keyboard shortcuts" size="lg">
      <ul className={styles.list}>
        {bound.map((a) => (
          <li key={a.id} className={styles.row}>
            <span>{a.title}</span>
            <kbd className={styles.kbd}>{formatShortcut(a.shortcut!)}</kbd>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}
