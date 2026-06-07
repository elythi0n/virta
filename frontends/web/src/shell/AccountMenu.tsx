import { useCallback, useState } from 'react';
import { Button, Dialog, Input, Popover, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { joinChannel } from '../daemon';
import { login, logout, register } from '../daemon/account';
import type { VirtaUser } from '../daemon/account';
import { clearGuestChannels, loadGuestChannels } from '../daemon/localChannels';
import { useHostedAuth } from '../daemon/hostedAuth';
import styles from './AccountMenu.module.css';

type Mode = 'idle' | 'login' | 'register';

// AccountMenu is shown in the titlebar in hosted mode only. In local mode (hosted=false) it
// renders nothing. In hosted mode with no session it shows a "Sign in" button; once the user
// logs in it shows their avatar + display name with a sign-out option.
export default function AccountMenu() {
  const { hosted, user, setUser } = useHostedAuth();
  const [mode, setMode] = useState<Mode>('idle');
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);

  // After a successful login/register, sync any guest channels to the server account
  // then clear local storage so there's no stale state.
  const syncGuestChannels = useCallback(async () => {
    const guests = loadGuestChannels();
    if (guests.length === 0) return;
    await Promise.allSettled(guests.map(c => joinChannel(c.platform, c.slug)));
    clearGuestChannels();
  }, []);

  const doLogin = useCallback(async () => {
    if (!email || !password) return;
    setBusy(true); setError('');
    try {
      const res = await login(email, password);
      const loggedIn = (res as { user: VirtaUser }).user;
      await syncGuestChannels();
      setUser(loggedIn);
      setMode('idle'); setEmail(''); setPassword('');
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Login failed');
    } finally {
      setBusy(false);
    }
  }, [email, password, setUser, syncGuestChannels]);

  const doRegister = useCallback(async () => {
    if (!email || !password) return;
    setBusy(true); setError('');
    try {
      const res = await register(email, name, password);
      const loggedIn = (res as { user: VirtaUser }).user;
      await syncGuestChannels();
      setUser(loggedIn);
      setMode('idle'); setEmail(''); setName(''); setPassword('');
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Registration failed');
    } finally {
      setBusy(false);
    }
  }, [email, name, password, setUser, syncGuestChannels]);

  const doLogout = useCallback(async () => {
    await logout().catch(() => {});
    setUser(null);
    setMenuOpen(false);
  }, [setUser]);

  if (!hosted) return null;

  return (
    <>
      {user ? (
        <Popover
          open={menuOpen}
          onOpenChange={setMenuOpen}
          align="end"
          side="bottom"
          trigger={
            <button type="button" className={styles.avatarBtn} aria-label="Account menu">
              <span className={styles.avatar}>
                {((user.display_name || user.email || '?')[0]).toUpperCase()}
              </span>
              <span className={styles.displayName}>{user.display_name || user.email.split('@')[0] || 'Account'}</span>
              <Icon name="chevron-down" size={12} />
            </button>
          }
        >
          <div className={styles.dropdown}>
            <div className={styles.dropdownUser}>
              <Text variant="ui" className={styles.dropName}>{user.display_name || 'Account'}</Text>
              <Text variant="meta" tone="subtle">{user.email}</Text>
            </div>
            <div className={styles.dropDivider} />
            <button type="button" className={styles.dropItem} onClick={() => { setMenuOpen(false); void doLogout(); }}>
              <Icon name="x" size={14} /> Sign out
            </button>
          </div>
        </Popover>
      ) : (
        <button type="button" className={styles.signInBtn} onClick={() => setMode('login')}>
          Sign in
        </button>
      )}

      <Dialog
        open={mode === 'login'}
        onOpenChange={o => !o && setMode('idle')}
        title="Sign in to Virta"
        description="Your channels, profiles, and history — synced across devices."
        size="sm"
        footer={
          <>
            <Button variant="ghost" size="md" onClick={() => setMode('idle')}>Cancel</Button>
            <Button variant="solid" size="md" disabled={busy || !email || !password} onClick={() => void doLogin()}>
              {busy ? 'Signing in…' : 'Sign in'}
            </Button>
          </>
        }
      >
        <div className={styles.authForm}>
          <label className={styles.fieldLabel}>
            Email
            <Input aria-label="Email" type="email" value={email} onChange={e => setEmail(e.currentTarget.value)}
              onKeyDown={e => e.key === 'Enter' && void doLogin()} autoFocus />
          </label>
          <label className={styles.fieldLabel}>
            Password
            <Input aria-label="Password" type="password" value={password} onChange={e => setPassword(e.currentTarget.value)}
              onKeyDown={e => e.key === 'Enter' && void doLogin()} />
          </label>
          {error && <Text variant="meta" tone="subtle" as="p" className={styles.authError}>{error}</Text>}
          <button type="button" className={styles.switchLink} onClick={() => { setMode('register'); setError(''); }}>
            No account? Create one
          </button>
        </div>
      </Dialog>

      <Dialog
        open={mode === 'register'}
        onOpenChange={o => !o && setMode('idle')}
        title="Create your Virta account"
        description="Your channels, chat history, and settings — synced and backed up."
        size="sm"
        footer={
          <>
            <Button variant="ghost" size="md" onClick={() => setMode('idle')}>Cancel</Button>
            <Button variant="solid" size="md" disabled={busy || !email || !password} onClick={() => void doRegister()}>
              {busy ? 'Creating…' : 'Create account'}
            </Button>
          </>
        }
      >
        <div className={styles.authForm}>
          <label className={styles.fieldLabel}>
            Email
            <Input aria-label="Email" type="email" value={email} onChange={e => setEmail(e.currentTarget.value)} autoFocus />
          </label>
          <label className={styles.fieldLabel}>
            Display name <span className={styles.hint}>(optional)</span>
            <Input aria-label="Display name" value={name} onChange={e => setName(e.currentTarget.value)} placeholder="How you appear to others" />
          </label>
          <label className={styles.fieldLabel}>
            Password <span className={styles.hint}>(min 8 characters)</span>
            <Input aria-label="Password" type="password" value={password} onChange={e => setPassword(e.currentTarget.value)}
              onKeyDown={e => e.key === 'Enter' && void doRegister()} />
          </label>
          {error && <Text variant="meta" tone="subtle" as="p" className={styles.authError}>{error}</Text>}
          <button type="button" className={styles.switchLink} onClick={() => { setMode('login'); setError(''); }}>
            Already have an account? Sign in
          </button>
        </div>
      </Dialog>
    </>
  );
}
