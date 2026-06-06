import { useCallback, useEffect, useState } from 'react';
import { Button, Segmented, Text } from '@virta/ui-kit';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import { disconnectAccount, listAccounts, listMethods, setMethod, useCapabilities } from '../daemon';
import type { AccountInfo } from '../daemon/wire.gen';
import SignInDialog, { type SignInPlatform } from '../shell/SignInDialog';
import styles from './Connections.module.css';

type PlatformDef = {
  id: Platform;
  label: string;
  signable: boolean;
  note: string;
};

const PLATFORMS: PlatformDef[] = [
  { id: 'twitch', label: 'Twitch', signable: true, note: 'Reads anonymously. Sign in to send and moderate from your account.' },
  { id: 'kick', label: 'Kick', signable: true, note: 'Reads anonymously (unofficial). Sign in to send and moderate.' },
  { id: 'x', label: 'X', signable: false, note: 'Best-effort read via a browser session — no official API.' },
];

const STABILITY: Record<string, string> = {
  official: 'Official API',
  unofficial: 'Unofficial',
  besteffort: 'Best-effort',
};

// The Connections surface (ADR-025): one card per platform showing what it can do (capability
// tags), how stable that access is, and the account state — sign in to unlock send/moderation.
// Reading needs no account, so a platform is useful before you ever sign in.
export default function Connections() {
  const { caps, status, refresh } = useCapabilities();
  const [signIn, setSignIn] = useState<SignInPlatform | null>(null);
  const [methods, setMethods] = useState<Record<string, string>>({});
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);

  const reloadAccounts = useCallback(() => {
    listAccounts()
      .then(setAccounts)
      .catch(() => {});
  }, []);

  useEffect(() => {
    listMethods()
      .then(setMethods)
      .catch(() => {});
    reloadAccounts();
  }, [reloadAccounts]);

  const disconnect = async (id: string) => {
    try {
      await disconnectAccount(id);
    } catch {
      // ignore; the reloads below reflect the real state
    }
    reloadAccounts();
    void refresh();
  };

  const changeMethod = async (platform: string, method: string) => {
    setMethods((m) => ({ ...m, [platform]: method })); // optimistic
    try {
      setMethods(await setMethod(platform, method));
    } catch {
      void listMethods().then(setMethods); // reload on failure
    }
    void refresh(); // capabilities shift with the method (e.g. anonymous drops send)
  };

  return (
    <div className={styles.list}>
      {status === 'offline' && (
        <Text variant="meta" tone="subtle" as="p" className={styles.offline}>
          Not connected to a daemon. Connection state shows here once it's running.
        </Text>
      )}

      {PLATFORMS.map((p) => {
        const c = caps[p.id];
        const account = accounts.find((a) => a.platform === p.id);
        const tags: { label: string; on: boolean }[] = [
          { label: 'Read', on: !!(c?.read_anonymous || c?.read_authed) },
          { label: 'Send', on: !!c?.send },
          { label: 'Moderate', on: !!c?.moderation },
          { label: 'Replies', on: !!c?.replies },
        ];
        return (
          <section key={p.id} className={styles.card}>
            <header className={styles.head}>
              <PlatformGlyph platform={p.id} className={styles.glyph} />
              <span className={styles.name}>{p.label}</span>
              {c?.stability && <span className={styles.stability}>{STABILITY[c.stability] ?? c.stability}</span>}
              <span className={styles.account}>
                {p.signable ? (
                  account ? (
                    <>
                      <span className={styles.identity}>{account.display_name || account.login}</span>
                      <Button variant="ghost" size="sm" onClick={() => void disconnect(account.id)}>
                        Disconnect
                      </Button>
                    </>
                  ) : (
                    <Button variant="subtle" size="sm" onClick={() => setSignIn(p.id as SignInPlatform)}>
                      Sign in
                    </Button>
                  )
                ) : (
                  <Text variant="meta" tone="subtle">
                    Read-only
                  </Text>
                )}
              </span>
            </header>

            <div className={styles.tags}>
              {tags.map((t) => (
                <span key={t.label} className={styles.tag} data-on={t.on}>
                  {t.label}
                </span>
              ))}
            </div>

            {account?.scopes && account.scopes.length > 0 && (
              <Text variant="meta" tone="subtle" as="p" className={styles.scopes}>
                Scopes: {account.scopes.join(', ')}
              </Text>
            )}

            {p.signable && (
              <div className={styles.method}>
                <span className={styles.methodLabel}>Connect</span>
                <Segmented
                  ariaLabel={`${p.label} connection method`}
                  value={methods[p.id] || 'automatic'}
                  onValueChange={(v) => void changeMethod(p.id, v)}
                  options={
                    c?.send
                      ? [
                          { value: 'automatic', label: 'Automatic' },
                          { value: 'anonymous', label: 'Anonymous' },
                          { value: 'authenticated', label: 'Authenticated' },
                        ]
                      : [
                          { value: 'automatic', label: 'Automatic' },
                          { value: 'anonymous', label: 'Anonymous' },
                        ]
                  }
                />
              </div>
            )}

            <Text variant="meta" tone="subtle" as="p" className={styles.note}>
              {p.note}
            </Text>
          </section>
        );
      })}

      <SignInDialog
        platform={signIn}
        onClose={() => setSignIn(null)}
        onAuthorized={() => {
          void refresh();
          reloadAccounts();
        }}
      />
    </div>
  );
}
