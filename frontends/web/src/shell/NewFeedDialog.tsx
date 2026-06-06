import { useEffect, useState } from 'react';
import { Button, Dialog, Input, Text } from '@virta/ui-kit';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import { useChannels } from '../daemon';
import styles from './NewFeedDialog.module.css';

type Props = {
  open: boolean;
  onClose: () => void;
  onSubmit: (title: string, channels: string[]) => void;
  /** Pre-fill when editing an existing feed. */
  initialName?: string;
  initialChannels?: string[];
  dialogTitle?: string;
  submitLabel?: string;
};

// Build or edit a feed scoped to a chosen set of joined channels — the basis for several unified
// feeds at once. An empty selection isn't allowed (that's just the default unified feed).
export default function NewFeedDialog({
  open,
  onClose,
  onSubmit,
  initialName,
  initialChannels,
  dialogTitle = 'New feed',
  submitLabel = 'Create feed',
}: Props) {
  const { channels } = useChannels();
  const [name, setName] = useState('');
  const [selected, setSelected] = useState<string[]>([]);

  // Seed the form from the initial values each time the dialog opens.
  useEffect(() => {
    if (open) {
      setName(initialName ?? '');
      setSelected(initialChannels ?? []);
    }
  }, [open, initialName, initialChannels]);

  const toggle = (key: string) =>
    setSelected((s) => (s.includes(key) ? s.filter((k) => k !== key) : [...s, key]));

  const submit = () => {
    if (selected.length === 0) return;
    const fallback = selected.length === 1 ? (selected[0].split(':')[1] ?? 'Feed') : `${selected.length} channels`;
    onSubmit(name.trim() || fallback, selected);
    onClose();
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={dialogTitle}
      description="A unified feed scoped to the channels you pick."
      footer={
        <>
          <Button variant="ghost" size="md" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="solid" size="md" disabled={selected.length === 0} onClick={submit}>
            {submitLabel}
          </Button>
        </>
      }
    >
      <div className={styles.body}>
        <Input
          placeholder="Feed name (optional)"
          aria-label="Feed name"
          value={name}
          onChange={(e) => setName(e.currentTarget.value)}
        />
        {channels.length === 0 ? (
          <Text tone="subtle" variant="ui" as="p">
            Join channels in Sources first, then build a feed from them.
          </Text>
        ) : (
          <ul className={styles.list}>
            {channels.map((c) => {
              const key = `${c.platform}:${c.slug}`;
              const on = selected.includes(key);
              return (
                <li key={key}>
                  <button
                    type="button"
                    className={`${styles.item} ${on ? styles.on : ''}`}
                    aria-pressed={on}
                    onClick={() => toggle(key)}
                  >
                    <span className={styles.check}>{on ? '✓' : ''}</span>
                    <PlatformGlyph platform={c.platform as Platform} className={styles.glyph} />
                    {c.slug}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </Dialog>
  );
}
