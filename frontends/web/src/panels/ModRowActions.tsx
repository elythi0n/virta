import type { FeedMessage } from '@virta/feed-core';
import { Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import styles from './ModRowActions.module.css';

export type ModAction = 'delete' | 'timeout' | 'ban';

// Hover actions on a chat row for a moderator: delete the message, time the user out, or ban them.
// The parent runs the action (it composes the slash command and sends it to the row's channel).
export default function ModRowActions({ message, onAction }: { message: FeedMessage; onAction: (action: ModAction, m: FeedMessage) => void }) {
  const btn = (action: ModAction, icon: 'trash' | 'clock' | 'ban', label: string, danger?: boolean) => (
    <Tooltip content={label} side="top">
      <button
        type="button"
        className={`${styles.btn} ${danger ? styles.danger : ''}`}
        aria-label={`${label} ${message.author}`}
        onClick={() => onAction(action, message)}
      >
        <Icon name={icon} size={14} />
      </button>
    </Tooltip>
  );
  return (
    <span className={styles.actions}>
      {message.platformMessageId && btn('delete', 'trash', 'Delete message')}
      {btn('timeout', 'clock', 'Time out')}
      {btn('ban', 'ban', 'Ban', true)}
    </span>
  );
}
