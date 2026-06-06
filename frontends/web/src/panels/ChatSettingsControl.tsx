import { useState } from 'react';
import { Popover, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import type { ChatSettings } from '../daemon/wire.gen';
import styles from './ChatSettingsControl.module.css';

// Followers-only is "on" when a minutes value is set (the daemon uses a negative value for off).
const followersOn = (s: ChatSettings) => s.followers_only_minutes >= 0;

// The slash command that toggles each setting on/off, given the current settings.
function command(setting: 'emote' | 'unique' | 'slow' | 'followers', s: ChatSettings): string {
  switch (setting) {
    case 'emote':
      return s.emote_only ? '/emoteonlyoff' : '/emoteonly';
    case 'unique':
      return s.unique_chat ? '/uniquechatoff' : '/uniquechat';
    case 'slow':
      return s.slow_seconds > 0 ? '/slowoff' : '/slow 30';
    case 'followers':
      return followersOn(s) ? '/followersoff' : '/followers';
  }
}

// Chat-settings quick toggles for a single moderatable channel: slow / followers-only / emote-only
// / unique-chat, reflecting the live state and toggled by sending the matching slash command.
export default function ChatSettingsControl({ settings, onCommand }: { settings?: ChatSettings; onCommand: (cmd: string) => void }) {
  const [open, setOpen] = useState(false);
  const s = settings ?? { emote_only: false, subs_only: false, unique_chat: false, followers_only_minutes: -1, slow_seconds: 0 };
  const toggles: { key: 'emote' | 'unique' | 'slow' | 'followers'; label: string; on: boolean }[] = [
    { key: 'emote', label: 'Emote-only', on: s.emote_only },
    { key: 'followers', label: 'Followers-only', on: followersOn(s) },
    { key: 'slow', label: 'Slow mode', on: s.slow_seconds > 0 },
    { key: 'unique', label: 'Unique chat', on: s.unique_chat },
  ];
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      align="end"
      trigger={
        <Tooltip content="Chat settings" side="bottom">
          <button type="button" className={styles.trigger} aria-label="Chat settings">
            <Icon name="mods" size={16} />
          </button>
        </Tooltip>
      }
    >
      <div className={styles.menu} role="group" aria-label="Chat settings">
        {toggles.map((t) => (
          <button
            key={t.key}
            type="button"
            className={`${styles.item} ${t.on ? styles.on : ''}`}
            aria-pressed={t.on}
            onClick={() => onCommand(command(t.key, s))}
          >
            <span className={styles.dot} data-on={t.on} aria-hidden />
            {t.label}
          </button>
        ))}
      </div>
    </Popover>
  );
}
