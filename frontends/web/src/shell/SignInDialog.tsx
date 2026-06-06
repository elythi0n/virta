import { useEffect, useRef, useState } from 'react';
import { Button, Dialog, Text } from '@virta/ui-kit';
import { kickAuthStatus, pollSession, startKickAuth, startTwitchDevice, twitchDeviceStatus } from '../daemon';
import type { DeviceSession } from '../daemon/wire.gen';
import styles from './SignInDialog.module.css';

export type SignInPlatform = 'twitch' | 'kick';

type Props = {
  platform: SignInPlatform | null;
  onClose: () => void;
  onAuthorized: () => void;
};

type Phase = 'starting' | 'pending' | 'authorized' | 'error';

// Drives a platform sign-in to completion. Twitch shows a device code to enter at the verification
// URL and polls; Kick opens its authorize URL in the browser and polls. Both stop on a terminal
// state. The daemon holds the tokens (keychain) — nothing secret is handled here.
export default function SignInDialog({ platform, onClose, onAuthorized }: Props) {
  const [phase, setPhase] = useState<Phase>('starting');
  const [device, setDevice] = useState<DeviceSession | null>(null);
  const [message, setMessage] = useState('');
  const onAuthorizedRef = useRef(onAuthorized);
  onAuthorizedRef.current = onAuthorized;

  useEffect(() => {
    if (!platform) return;
    setPhase('starting');
    setDevice(null);
    setMessage('');
    const ctrl = new AbortController();

    const settle = (state: string, error?: string) => {
      if (state === 'authorized') {
        setPhase('authorized');
        onAuthorizedRef.current();
      } else {
        setPhase('error');
        setMessage(error || `Sign-in ${state}.`);
      }
    };

    void (async () => {
      try {
        if (platform === 'twitch') {
          const session = await startTwitchDevice();
          setDevice(session);
          setPhase('pending');
          const final = await pollSession(() => twitchDeviceStatus(session.id), {
            intervalMs: (session.interval || 5) * 1000,
            signal: ctrl.signal,
          });
          settle(final.state, final.error);
        } else {
          const session = await startKickAuth();
          setPhase('pending');
          setMessage('Continue in your browser, then return here.');
          window.open(session.authorize_url, '_blank', 'noopener,noreferrer');
          const final = await pollSession(() => kickAuthStatus(session.id), { intervalMs: 2000, signal: ctrl.signal });
          settle(final.state, final.error);
        }
      } catch (e) {
        if (e instanceof DOMException && e.name === 'AbortError') return;
        setPhase('error');
        setMessage(e instanceof Error ? e.message : String(e));
      }
    })();

    return () => ctrl.abort();
  }, [platform]);

  const title = platform === 'kick' ? 'Sign in to Kick' : 'Sign in to Twitch';

  return (
    <Dialog open={platform !== null} onOpenChange={(open) => !open && onClose()} title={title}>
      {phase === 'starting' && (
        <Text tone="subtle" as="p">
          Starting sign-in…
        </Text>
      )}

      {phase === 'pending' && platform === 'twitch' && device && (
        <div className={styles.flow}>
          <Text as="p">
            Go to{' '}
            <a className={styles.link} href={device.verification_uri} target="_blank" rel="noreferrer">
              {device.verification_uri}
            </a>{' '}
            and enter this code:
          </Text>
          <div className={styles.code}>{device.user_code}</div>
          <Button variant="subtle" size="sm" onClick={() => void navigator.clipboard?.writeText(device.user_code)}>
            Copy code
          </Button>
          <Text tone="subtle" variant="meta" as="p">
            Waiting for authorization…
          </Text>
        </div>
      )}

      {phase === 'pending' && platform === 'kick' && (
        <Text tone="subtle" as="p">
          {message || 'Opening your browser…'}
        </Text>
      )}

      {phase === 'authorized' && (
        <Text as="p">Signed in. You can close this window.</Text>
      )}

      {phase === 'error' && <p className={styles.error}>{message}</p>}
    </Dialog>
  );
}
