import { Command } from 'cmdk';
import { formatShortcut } from './shortcut';
import './CommandPalette.css';

export interface CommandAction {
  id: string;
  title: string;
  /** Section heading the action is grouped under. */
  group?: string;
  /** Extra terms to match on beyond the title. */
  keywords?: string[];
  /** Keyboard shortcut spec (e.g. 'mod+b'); shown in the palette and dispatched by the keymap. */
  shortcut?: string;
  perform: () => void;
}

type CommandPaletteProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  actions: CommandAction[];
  placeholder?: string;
};

// VS Code-style command palette on cmdk (headless: filtering, keyboard nav, a11y); the chrome is
// ours, token-styled. Actions are registered by the app and grouped by `group`.
export default function CommandPalette({ open, onOpenChange, actions, placeholder }: CommandPaletteProps) {
  const groups = new Map<string, CommandAction[]>();
  for (const a of actions) {
    const key = a.group ?? 'Commands';
    const list = groups.get(key);
    if (list) list.push(a);
    else groups.set(key, [a]);
  }

  return (
    <Command.Dialog open={open} onOpenChange={onOpenChange} label="Command palette">
      <Command.Input placeholder={placeholder ?? 'Type a command…'} />
      <Command.List>
        <Command.Empty>No matching commands</Command.Empty>
        {[...groups.entries()].map(([heading, items]) => (
          <Command.Group key={heading} heading={heading}>
            {items.map((a) => (
              <Command.Item
                key={a.id}
                value={`${a.title} ${(a.keywords ?? []).join(' ')}`}
                onSelect={() => {
                  onOpenChange(false);
                  a.perform();
                }}
              >
                <span>{a.title}</span>
                {a.shortcut && <kbd>{formatShortcut(a.shortcut)}</kbd>}
              </Command.Item>
            ))}
          </Command.Group>
        ))}
      </Command.List>
    </Command.Dialog>
  );
}
