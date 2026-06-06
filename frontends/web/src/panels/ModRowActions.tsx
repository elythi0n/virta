import type { FeedMessage } from '@virta/feed-core';
import { Tooltip } from '@virta/ui-kit';
import Icon, { type IconName } from '../Icon';
import styles from './ModRowActions.module.css';

export type ModAction = 'delete' | 'timeout' | 'ban';

// Hover actions on a chat row: reply (any signed-in viewer, when the platform supports it) and the
// moderator trio (delete / timeout / ban). The parent runs each action.
export default function ModRowActions({
  message,
  onReply,
  onModerate,
}: {
  message: FeedMessage;
  onReply?: (m: FeedMessage) => void;
  onModerate?: (action: ModAction, m: FeedMessage) => void;
}) {
  const btn = (icon: IconName, label: string, onClick: () => void, danger?: boolean) => (
    <Tooltip content={label} side="top">
      <button
        type="button"
        className={`${styles.btn} ${danger ? styles.danger : ''}`}
        aria-label={`${label} ${message.author}`}
        onClick={onClick}
      >
        <Icon name={icon} size={14} />
      </button>
    </Tooltip>
  );
  return (
    <span className={styles.actions}>
      {onReply && btn('reply', 'Reply', () => onReply(message))}
      {onModerate && message.platformMessageId && btn('trash', 'Delete message', () => onModerate('delete', message))}
      {onModerate && btn('clock', 'Time out', () => onModerate('timeout', message))}
      {onModerate && btn('ban', 'Ban', () => onModerate('ban', message), true)}
    </span>
  );
}
