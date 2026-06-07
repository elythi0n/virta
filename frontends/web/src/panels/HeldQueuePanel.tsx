import { Button, Text } from '@virta/ui-kit';
import { PlatformGlyph, type Platform } from '@virta/feed-core';
import Icon from '../Icon';
import { useHeld } from '../daemon';
import styles from './HeldQueuePanel.module.css';

const platformOf = (channel: string) => channel.split(':')[0] as Platform;
const slugOf = (channel: string) => channel.split(':')[1] ?? channel;

// The AutoMod hold queue: messages a platform is holding for review, each with approve (post) and
// deny (drop). The list is live (held / held_resolved events) and resolves optimistically.
export default function HeldQueuePanel() {
  const { held, approve, deny } = useHeld();

  if (held.length === 0) {
    return (
      <div className={styles.empty}>
        <Icon name="mods" size={22} />
        <Text variant="body" tone="subtle">
          Nothing held for review.
        </Text>
        <Text variant="body" tone="subtle">
          When Twitch AutoMod catches a message, it appears here for you to approve (post it) or
          deny (drop it). Enable AutoMod in your Twitch Creator Dashboard → Moderation settings.
        </Text>
      </div>
    );
  }

  return (
    <div className={styles.queue}>
      <div className={styles.head}>
        <Text variant="meta" tone="subtle">
          {held.length} held for review
        </Text>
      </div>
      <ul className={styles.list}>
        {held.map((h) => (
          <li key={h.id} className={styles.item}>
            <div className={styles.meta}>
              <PlatformGlyph platform={platformOf(h.channel)} className={styles.glyph} />
              <span className={styles.channel}>{slugOf(h.channel)}</span>
              <span className={styles.author}>{h.author}</span>
              {h.reason && <span className={styles.reason}>{h.reason}</span>}
            </div>
            <div className={styles.body}>{h.text}</div>
            <div className={styles.actions}>
              <Button variant="ghost" size="sm" onClick={() => deny(h.id)}>
                Deny
              </Button>
              <Button variant="solid" size="sm" onClick={() => approve(h.id)}>
                Approve
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
