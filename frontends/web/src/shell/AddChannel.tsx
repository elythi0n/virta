import { useState } from 'react';
import { Button, Input, Popover, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { useChannels } from '../daemon';
import styles from './AddChannel.module.css';

const PLATFORMS = ['twitch', 'kick'] as const;
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

// A "+" in the Streams header: pick a platform, type a channel, and join it. The streams rail
// (polling the daemon) picks the new channel up within a few seconds.
export default function AddChannel() {
  const { join } = useChannels();
  const [open, setOpen] = useState(false);
  const [platform, setPlatform] = useState<string>('twitch');
  const [slug, setSlug] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onJoin = async () => {
    const name = slug.trim().toLowerCase();
    if (!name) return;
    setBusy(true);
    setError(null);
    try {
      await join(platform, name);
      setSlug('');
      setOpen(false);
    } catch {
      setError(`Couldn’t add ${cap(platform)}/${name}.`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (!o) setError(null);
      }}
      align="end"
      trigger={
        <Tooltip content="Add a channel" side="bottom">
          <button type="button" className={styles.add} aria-label="Add a channel">
            <Icon name="plus" size={20} />
          </button>
        </Tooltip>
      }
    >
      <div className={styles.form}>
        <div className={styles.platforms} role="group" aria-label="Platform">
          {PLATFORMS.map((p) => (
            <button
              key={p}
              type="button"
              className={`${styles.chip} ${platform === p ? styles.chipOn : ''}`}
              aria-pressed={platform === p}
              onClick={() => setPlatform(p)}
            >
              {cap(p)}
            </button>
          ))}
        </div>
        <div className={styles.entry}>
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
            Add
          </Button>
        </div>
        {error && <span className={styles.error}>{error}</span>}
      </div>
    </Popover>
  );
}
