import { useEffect, useState } from 'react';
import { Button, Input, Text } from '@virta/ui-kit';
import { previewSend, sendMessage } from '../daemon';
import type { SendTarget } from '../daemon/wire.gen';
import styles from './Composer.module.css';

const label = (channel: string) => channel.split(':')[1] ?? channel;

type Props = {
  /** Channel keys ("platform:slug") this composer can post to. */
  targets: string[];
};

// Compose and cross-post to the feed's channels. Previews reachability so unreachable targets show
// as dimmed ⊘ chips with a passive "won't reach" line, and only reachable ones receive the message.
export default function Composer({ targets }: Props) {
  const [text, setText] = useState('');
  const [preview, setPreview] = useState<SendTarget[] | null>([]); // null = daemon unreachable
  const [sending, setSending] = useState(false);
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
  }, [key]);

  const reachable = (preview ?? []).filter((t) => t.can_send);
  const unreachable = (preview ?? []).filter((t) => !t.can_send);
  const offline = preview === null;
  const canSend = reachable.length > 0 && text.trim() !== '' && !sending;

  const submit = async () => {
    if (!canSend) return;
    setSending(true);
    try {
      await sendMessage(reachable.map((r) => r.channel), text.trim());
      setText('');
    } catch {
      // a transient failure; the feed will reflect what actually sent
    } finally {
      setSending(false);
    }
  };

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
            <span key={t.channel} className={`${styles.chip} ${styles.off}`} title={t.reason || 'not reachable'}>
              ⊘ {label(t.channel)}
            </span>
          ))}
        </div>
      )}
      <div className={styles.row}>
        <Input
          placeholder={offline ? 'Not connected' : reachable.length === 0 ? 'Sign in to send' : 'Send a message'}
          aria-label="Message"
          value={text}
          disabled={offline || reachable.length === 0}
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
      </div>
      {unreachable.length > 0 && (
        <Text variant="meta" tone="subtle" as="p" className={styles.note}>
          Won&rsquo;t reach {unreachable.map((t) => label(t.channel)).join(', ')} — sign in from Sources.
        </Text>
      )}
    </div>
  );
}
