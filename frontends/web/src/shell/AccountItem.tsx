import { useCallback, useState } from 'react';
import { Button, Dialog, Input, Popover, Text, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { login, logout, register } from '../daemon/account';
import { clearGuestChannels, joinChannel, loadGuestChannels } from '../daemon';
import { useHostedAuth } from '../daemon/hostedAuth';
import styles from './AccountItem.module.css';

type Mode = 'idle' | 'login' | 'register';

// Sidebar account button: sits above the settings item in the activity bar.
// In local mode (hosted=false) it renders nothing.
// When logged in: shows a coloured avatar that opens a popover to the right.
// When not logged in: shows a user icon that opens the sign-in dialog.
export default function AccountItem() {
  const { hosted, user, setUser } = useHostedAuth();
  const [mode, setMode] = useState<Mode>('idle');
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const syncGuest = useCallback(async () => {
    const guests = loadGuestChannels();
    if (guests.length > 0) {
      await Promise.allSettled(guests.map(c => joinChannel(c.platform, c.slug)));
      clearGuestChannels();
    }
  }, []);

  const doLogin = useCallback(async () => {
    if (!email || !password) return;
    setBusy(true); setError('');
    try {
      const res = await login(email, password);
      await syncGuest();
      setUser(res.user);
      setMode('idle'); setEmail(''); setPassword('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Login failed');
    } finally { setBusy(false); }
  }, [email, password, setUser, syncGuest]);

  const doRegister = useCallback(async () => {
    if (!email || !password) return;
    setBusy(true); setError('');
    try {
      const res = await register(email, name, password);
      await syncGuest();
      setUser(res.user);
      setMode('idle'); setEmail(''); setName(''); setPassword('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Registration failed');
    } finally { setBusy(false); }
  }, [email, name, password, setUser, syncGuest]);

  const doLogout = useCallback(async () => {
    await logout().catch(() => {});
    setUser(null);
  }, [setUser]);

  if (!hosted) return null;

  // Logged-in state: coloured avatar opens a right-side popover.
  if (user) {
    const initial = ((user.display_name || user.email || '?')[0]).toUpperCase();
    const label = user.display_name || user.email.split('@')[0] || 'Account';
    return (
      <Popover
        side="right"
        align="end"
        trigger={
          <button className={styles.btn} aria-label={`Account: ${label}`} title={label}>
            <span className={styles.avatar}>{initial}</span>
          </button>
        }
      >
        <div className={styles.menu}>
          <div className={styles.menuUser}>
            <span className={styles.menuName}>{user.display_name || 'Account'}</span>
            <Text variant="meta" tone="subtle" as="p" className={styles.menuEmail}>{user.email}</Text>
          </div>
          <div className={styles.menuDivider} />
          <button type="button" className={styles.menuItem} onClick={() => void doLogout()}>
            <Icon name="x" size={13} />
            Sign out
          </button>
        </div>
      </Popover>
    );
  }

  // Guest state: icon button opens the sign-in dialog.
  return (
    <>
      <Tooltip content="Sign in" side="right">
        <button
          type="button"
          className={`${styles.btn} ${styles.btnGuest}`}
          aria-label="Sign in"
          onClick={() => setMode('login')}
        >
          <Icon name="user" size={20} />
        </button>
      </Tooltip>

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
        <div className={styles.form}>
          <label className={styles.field}>
            Email
            <Input aria-label="Email" type="email" value={email}
              onChange={e => setEmail(e.currentTarget.value)}
              onKeyDown={e => e.key === 'Enter' && void doLogin()} autoFocus />
          </label>
          <label className={styles.field}>
            Password
            <Input aria-label="Password" type="password" value={password}
              onChange={e => setPassword(e.currentTarget.value)}
              onKeyDown={e => e.key === 'Enter' && void doLogin()} />
          </label>
          {error && <p className={styles.formError}>{error}</p>}
          <button type="button" className={styles.switchLink}
            onClick={() => { setMode('register'); setError(''); }}>
            No account? Create one
          </button>
        </div>
      </Dialog>

      <Dialog
        open={mode === 'register'}
        onOpenChange={o => !o && setMode('idle')}
        title="Create your Virta account"
        description="Channels, chat history, and settings — synced and backed up."
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
        <div className={styles.form}>
          <label className={styles.field}>
            Email
            <Input aria-label="Email" type="email" value={email}
              onChange={e => setEmail(e.currentTarget.value)} autoFocus />
          </label>
          <label className={styles.field}>
            Display name <span className={styles.hint}>(optional)</span>
            <Input aria-label="Display name" value={name}
              onChange={e => setName(e.currentTarget.value)}
              placeholder="How you appear to others" />
          </label>
          <label className={styles.field}>
            Password <span className={styles.hint}>(min 8 characters)</span>
            <Input aria-label="Password" type="password" value={password}
              onChange={e => setPassword(e.currentTarget.value)}
              onKeyDown={e => e.key === 'Enter' && void doRegister()} />
          </label>
          {error && <p className={styles.formError}>{error}</p>}
          <button type="button" className={styles.switchLink}
            onClick={() => { setMode('login'); setError(''); }}>
            Already have an account? Sign in
          </button>
        </div>
      </Dialog>
    </>
  );
}
