import { useEffect, useState } from 'react';
import { Button, Input, Popover, Text } from '@virta/ui-kit';
import { previewSend, sendMessage } from '../daemon';
import SignInDialog, { type SignInPlatform } from '../shell/SignInDialog';
import type { SendTarget } from '../daemon/wire.gen';
import styles from './Composer.module.css';

const SIGNABLE = new Set(['twitch', 'kick']);
const platformOf = (channel: string) => channel.split(':')[0];
const label = (channel: string) => channel.split(':')[1] ?? channel;
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

type Props = {
  /** Channel keys ("platform:slug") this composer can post to. */
  targets: string[];
};

// Compose and cross-post to the feed's channels. Reachable targets show as chips; unreachable ones
// (platforms you're not signed in to) are dimmed, and a "Sign in to chat" action opens the auth
// modal for those platforms. Messages only ever go to platforms you're signed in to.
export default function Composer({ targets }: Props) {
  const [text, setText] = useState('');
  const [preview, setPreview] = useState<SendTarget[] | null>([]); // null = daemon unreachable
  const [sending, setSending] = useState(false);
  const [signIn, setSignIn] = useState<SignInPlatform | null>(null);
  const [reload, setReload] = useState(0);
  const key = [...targets].sort().join(',');

  useEffect(() => {
    if (targets.length === 0) {
      setPreview([]);
      return;
    }
    let cancelled = false;
    previewSend(targets)
      .then((t) => !cancelled && setPreview(t))
      .catch(() => !cancelled && setPreview(null));
    return () => {
      cancelled = true;
    };
  }, [key, reload]);

  const reachable = (preview ?? []).filter((t) => t.can_send);
  const unreachable = (preview ?? []).filter((t) => !t.can_send);
  const offline = preview === null;
  const signable = [...new Set(unreachable.map((t) => platformOf(t.channel)).filter((p) => SIGNABLE.has(p)))] as SignInPlatform[];
  const canSend = reachable.length > 0 && text.trim() !== '' && !sending;

  const submit = async () => {
    if (!canSend) return;
    setSending(true);
    try {
      await sendMessage(reachable.map((r) => r.channel), text.trim());
      setText('');
    } catch {
      // a transient failure; the feed reflects what actually sent
    } finally {
      setSending(false);
    }
  };

  function signInControl() {
    if (signable.length === 0) return null;
    const primary = reachable.length === 0;
    const variant = primary ? 'solid' : 'subtle';
    if (signable.length === 1) {
      return (
        <Button variant={variant} size="md" onClick={() => setSignIn(signable[0])}>
          {primary ? 'Sign in to chat' : `Sign in to ${cap(signable[0])}`}
        </Button>
      );
    }
    return (
      <Popover
        align="end"
        trigger={
          <Button variant={variant} size="md">
            {primary ? 'Sign in to chat' : 'Sign in'}
          </Button>
        }
      >
        <div className={styles.signinMenu}>
          {signable.map((p) => (
            <button key={p} type="button" className={styles.signinItem} onClick={() => setSignIn(p)}>
              Sign in to {cap(p)}
            </button>
          ))}
        </div>
      </Popover>
    );
  }

  return (
    <div className={styles.composer}>
      {preview && preview.length > 0 && (
        <div className={styles.chips}>
          {reachable.map((t) => (
            <span key={t.channel} className={styles.chip}>
              {label(t.channel)}
            </span>
          ))}
          {unreachable.map((t) => (
            <span key={t.channel} className={`${styles.chip} ${styles.off}`} title="Sign in to send here">
              ⊘ {label(t.channel)}
            </span>
          ))}
        </div>
      )}

      <div className={styles.row}>
        {reachable.length === 0 ? (
          (signInControl() ?? (
            <Input disabled placeholder={offline ? 'Not connected' : 'Sign in to chat'} aria-label="Message" />
          ))
        ) : (
          <>
            <Input
              placeholder="Send a message"
              aria-label="Message"
              value={text}
              onChange={(e) => setText(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  void submit();
                }
              }}
            />
            <Button variant="solid" size="md" disabled={!canSend} onClick={() => void submit()}>
              Send
            </Button>
            {signInControl()}
          </>
        )}
      </div>

      {unreachable.length > 0 && reachable.length > 0 && (
        <Text variant="meta" tone="subtle" as="p" className={styles.note}>
          Messages only go to platforms you&rsquo;re signed in to.
        </Text>
      )}

      <SignInDialog platform={signIn} onClose={() => setSignIn(null)} onAuthorized={() => setReload((r) => r + 1)} />
    </div>
  );
}
