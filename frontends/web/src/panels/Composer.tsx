import { useEffect, useState } from 'react';
import { Button, Input, Popover, Text } from '@virta/ui-kit';
import { previewSend, sendMessage } from '../daemon';
import SignInDialog, { type SignInPlatform } from '../shell/SignInDialog';
import type { SendTarget } from '../daemon/wire.gen';
import styles from './Composer.module.css';

const SIGNABLE = new Set(['twitch', 'kick']);
const platformOf = (channel: string) => channel.split(':')[0];
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

type Props = {
  /** Channel keys ("platform:slug") this composer can post to. */
  targets: string[];
};

// Compose and cross-post to the feed's channels. The input and Send button are always present;
// pressing Send when you're not signed in to any of the feed's platforms opens the sign-in
// options instead of sending (a dialog for one platform, a dropdown for several). Messages only
// ever go to platforms you're signed in to.
export default function Composer({ targets }: Props) {
  const [text, setText] = useState('');
  const [preview, setPreview] = useState<SendTarget[] | null>([]); // null = daemon unreachable
  const [sending, setSending] = useState(false);
  const [signIn, setSignIn] = useState<SignInPlatform | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
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

  const hasText = text.trim() !== '';
  // Send is actionable when there's text to send somewhere, or text plus a way to sign in.
  const canAct = hasText && !sending && (reachable.length > 0 || signable.length > 0);

  const send = async () => {
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

  // Pressing Send (or Enter): send if signed in somewhere, otherwise surface the sign-in options.
  const act = () => {
    if (!canAct) return;
    if (reachable.length > 0) {
      void send();
    } else if (signable.length === 1) {
      setSignIn(signable[0]);
    } else if (signable.length > 1) {
      setMenuOpen(true);
    }
  };

  const input = (
    <Input
      placeholder={offline ? 'Not connected' : 'Send a message'}
      aria-label="Message"
      disabled={offline}
      value={text}
      onChange={(e) => setText(e.currentTarget.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          act();
        }
      }}
    />
  );

  const sendButton = (
    <Button variant="solid" size="md" disabled={!canAct} onClick={act}>
      Send
    </Button>
  );

  return (
    <div className={styles.composer}>
      <div className={styles.row}>
        {input}
        {reachable.length === 0 && signable.length > 1 ? (
          // No account yet for any platform: Send opens a dropdown of sign-in options.
          <Popover
            open={menuOpen}
            onOpenChange={setMenuOpen}
            align="end"
            side="top"
            trigger={
              <Button variant="solid" size="md" disabled={!canAct}>
                Send
              </Button>
            }
          >
            <div className={styles.signinMenu} role="menu" aria-label="Sign in to send">
              {signable.map((p) => (
                <button
                  key={p}
                  type="button"
                  className={styles.signinItem}
                  onClick={() => {
                    setMenuOpen(false);
                    setSignIn(p);
                  }}
                >
                  Sign in to {cap(p)}
                </button>
              ))}
            </div>
          </Popover>
        ) : (
          sendButton
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
