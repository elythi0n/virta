import { useState } from 'react';
import { Button, Dialog, Input, Tooltip } from '@virta/ui-kit';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import Icon from '../Icon';
import { useChannels } from '../daemon';
import styles from './AddChannel.module.css';

const PLATFORMS = ['twitch', 'kick'] as const;
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

// The "+" in the Streams header opens a modal to add a stream: pick a platform, type a channel, and
// join it. The streams rail (polling the daemon) picks the new channel up within a few seconds, and
// if the streamer is already joined on another platform the two collapse into one card.
export default function AddChannel() {
  const { join } = useChannels();
  const [open, setOpen] = useState(false);
  const [platform, setPlatform] = useState<string>('twitch');
  const [slug, setSlug] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setSlug('');
    setError(null);
    setBusy(false);
  };

  const close = () => {
    setOpen(false);
    reset();
  };

  const onJoin = async () => {
    const name = slug.trim().toLowerCase();
    if (!name) return;
    setBusy(true);
    setError(null);
    try {
      await join(platform, name);
      close();
    } catch (e: unknown) {
      const detail = e instanceof Error ? e.message : '';
      setError(detail || `Couldn't add ${cap(platform)}/${name}. Check the channel name and try again.`);
      setBusy(false);
    }
  };

  return (
    <>
      <Tooltip content="Add a stream" side="bottom">
        <button type="button" className={styles.add} aria-label="Add a stream" onClick={() => setOpen(true)}>
          <Icon name="plus" size={20} />
        </button>
      </Tooltip>
      <Dialog
        open={open}
        onOpenChange={(o) => (o ? setOpen(true) : close())}
        title="Add a stream"
        description="Choose a platform and enter the channel name to join."
        footer={
          <>
            <Button variant="ghost" size="md" onClick={close}>
              Cancel
            </Button>
            <Button variant="solid" size="md" disabled={busy || slug.trim() === ''} onClick={() => void onJoin()}>
              Add stream
            </Button>
          </>
        }
      >
        <div className={styles.form}>
          <div className={styles.platforms} role="radiogroup" aria-label="Platform">
            {PLATFORMS.map((p) => {
              const on = platform === p;
              return (
                <button
                  key={p}
                  type="button"
                  role="radio"
                  aria-checked={on}
                  className={`${styles.platform} ${on ? styles.platformOn : ''}`}
                  onClick={() => setPlatform(p)}
                >
                  <PlatformGlyph platform={p as Platform} className={styles.platformGlyph} />
                  <span className={styles.platformName}>{cap(p)}</span>
                  {on && <Icon name="check" size={14} className={styles.platformCheck} />}
                </button>
              );
            })}
          </div>

          <div className={styles.field}>
            <label className={styles.label} htmlFor="add-channel-name">
              Channel name
            </label>
            <Input
              id="add-channel-name"
              aria-label="Channel name"
              placeholder={platform === 'kick' ? 'kick.com/channel' : 'twitch.tv/channel'}
              value={slug}
              autoFocus
              onChange={(e) => setSlug(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void onJoin();
              }}
            />
          </div>

          {error && <span className={styles.error}>{error}</span>}
        </div>
      </Dialog>
    </>
  );
}
