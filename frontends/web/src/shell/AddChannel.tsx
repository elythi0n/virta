import { useState } from 'react';
import { Button, Input, Popover, Select, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels } from '../daemon';
import styles from './AddChannel.module.css';

const PLATFORMS = [
  { value: 'twitch', label: 'Twitch' },
  { value: 'kick', label: 'Kick' },
];

// A compact "+" in the Streams header: pick a platform, type a channel, and join it. The streams
// rail (polling the daemon) picks the new channel up on its own.
export default function AddChannel() {
  const { join } = useChannels();
  const [open, setOpen] = useState(false);
  const [platform, setPlatform] = useState('twitch');
  const [slug, setSlug] = useState('');
  const [busy, setBusy] = useState(false);

  const onJoin = async () => {
    const name = slug.trim().toLowerCase();
    if (!name) return;
    setBusy(true);
    try {
      await join(platform, name);
      setSlug('');
      setOpen(false);
    } catch {
      // surfaced via the channel list's own error on the next refresh
    } finally {
      setBusy(false);
    }
  };

  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      align="end"
      trigger={
        <Tooltip content="Add a channel" side="bottom">
          <button type="button" className={styles.add} aria-label="Add a channel">
            <Icon name="plus" size={16} />
          </button>
        </Tooltip>
      }
    >
      <div className={styles.form}>
        <Select ariaLabel="Platform" value={platform} onValueChange={setPlatform} options={PLATFORMS} />
        <Input
          aria-label="Channel name"
          placeholder="channel"
          value={slug}
          autoFocus
          onChange={(e) => setSlug(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') void onJoin();
          }}
        />
        <Button variant="solid" size="sm" disabled={busy || slug.trim() === ''} onClick={() => void onJoin()}>
          Join
        </Button>
      </div>
    </Popover>
  );
}
