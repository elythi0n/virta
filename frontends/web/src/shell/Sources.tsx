import { useState } from 'react';
import { Badge, Button, Input, Select, StatusDot, Text, type DotStatus } from '@virta/ui-kit';
import { useCapabilities, useChannels } from '../daemon';
import SignInDialog, { type SignInPlatform } from './SignInDialog';
import styles from './Sources.module.css';

const PLATFORMS = [
  { value: 'twitch', label: 'Twitch' },
  { value: 'kick', label: 'Kick' },
];

const ACCOUNTS: { id: SignInPlatform; label: string }[] = [
  { id: 'twitch', label: 'Twitch' },
  { id: 'kick', label: 'Kick' },
];

function stateDot(state: string): DotStatus {
  if (state === 'connected') return 'live';
  if (state === 'error') return 'offline';
  return 'idle';
}

export default function Sources() {
  const { channels, status, error, join, leave, refresh } = useChannels();
  const { caps, refresh: refreshCaps } = useCapabilities();
  const [platform, setPlatform] = useState('twitch');
  const [slug, setSlug] = useState('');
  const [busy, setBusy] = useState(false);
  const [signIn, setSignIn] = useState<SignInPlatform | null>(null);

  const onJoin = async () => {
    const name = slug.trim().toLowerCase();
    if (!name) return;
    setBusy(true);
    try {
      await join(platform, name);
      setSlug('');
    } catch {
      // surfaced via the hook's error on the next refresh
    } finally {
      setBusy(false);
    }
  };

  if (status === 'offline') {
    return (
      <Text variant="ui" tone="subtle" as="p" className={styles.offline}>
        Not connected to a daemon. Launch the app (or run virtad) to add channels.
      </Text>
    );
  }

  return (
    <div className={styles.sources}>
      <section className={styles.section}>
        <Text variant="meta" tone="subtle" as="h3" className={styles.heading}>
          Accounts
        </Text>
        <ul className={styles.list}>
          {ACCOUNTS.map((a) => {
            const signedIn = caps[a.id]?.send ?? false;
            return (
              <li key={a.id} className={styles.row}>
                <span className={styles.rail} data-platform={a.id} />
                <Text variant="ui" tone="muted" className={styles.slug}>
                  {a.label}
                </Text>
                {signedIn ? (
                  <Badge tone="ok">Signed in</Badge>
                ) : (
                  <Button variant="subtle" size="sm" onClick={() => setSignIn(a.id)}>
                    Sign in
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
        <Text as="p" variant="meta" tone="subtle" className={styles.note}>
          Reading is anonymous; sign in to send and moderate.
        </Text>
      </section>

      <section className={styles.section}>
        <Text variant="meta" tone="subtle" as="h3" className={styles.heading}>
          Channels
        </Text>
        <div className={styles.add}>
          <Select ariaLabel="Platform" value={platform} onValueChange={setPlatform} options={PLATFORMS} />
          <Input
            aria-label="Channel name"
            placeholder="channel"
            value={slug}
            onChange={(e) => setSlug(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void onJoin();
            }}
          />
          <Button variant="solid" size="sm" disabled={busy || slug.trim() === ''} onClick={() => void onJoin()}>
            Join
          </Button>
        </div>

        {error && <p className={styles.error}>{error}</p>}

        <ul className={styles.list}>
          {channels.length === 0 ? (
            <li>
              <Text variant="meta" tone="subtle">
                No channels joined yet.
              </Text>
            </li>
          ) : (
            channels.map((c) => (
              <li key={`${c.platform}:${c.slug}`} className={styles.row}>
                <StatusDot status={stateDot(c.state)} label={c.state} />
                <Text variant="ui" tone="muted" className={styles.slug}>
                  {c.platform}/{c.slug}
                </Text>
                <button
                  type="button"
                  className={styles.remove}
                  aria-label={`Leave ${c.platform}/${c.slug}`}
                  onClick={() => void leave(c.platform, c.slug)}
                >
                  ×
                </button>
              </li>
            ))
          )}
        </ul>
      </section>

      <SignInDialog
        platform={signIn}
        onClose={() => setSignIn(null)}
        onAuthorized={() => {
          void refreshCaps();
          void refresh();
        }}
      />
    </div>
  );
}
