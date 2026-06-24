import { useEffect, useState } from 'react';
import type { IDockviewPanelHeaderProps } from 'dockview';
import { ContextMenu, type ContextMenuEntry } from '@virta/ui-kit';
import Icon, { type IconName } from '../Icon';
import { panelByKind } from '../panels/registry';
import { activityLevel, clearActivity, useActivityVersion } from '../dock/activity';
import styles from '../dock/Tab.module.css';

// Custom tab for the Broadcast workspace. Tabs whose panel was part of the seeded layout carry
// `params.locked = true` and render without a close affordance — they're part of the workspace
// itself. Tabs added by the user via "+" don't carry that flag, so a duplicate (e.g. a second
// chat dragged in by mistake) can still be removed.
//
// Rename + activity-dot behaviour mirror the main tab so the Broadcast workspace stays familiar.
const KIND_ICONS: Record<string, IconName> = {
  'stream-info': 'deck',
};

export default function DeckTab(props: IDockviewPanelHeaderProps) {
  const { api, params } = props;
  const [renaming, setRenaming] = useState(false);
  const [draft, setDraft] = useState('');

  const [isActive, setIsActive] = useState(api.isActive);
  useEffect(() => {
    const sub = api.onDidActiveChange((e) => {
      setIsActive(e.isActive);
      clearActivity(api.id);
    });
    return () => sub.dispose();
  }, [api]);
  useActivityVersion();
  const level = activityLevel(api.id);
  const showDot = !isActive && !!level;

  const kind = typeof params.kind === 'string' ? params.kind : undefined;
  const icon: IconName | undefined = kind
    ? KIND_ICONS[kind] ?? panelByKind(kind)?.icon
    : undefined;

  const locked = params.locked === true;

  const startRename = () => {
    setDraft(api.title ?? '');
    setRenaming(true);
  };
  const commitRename = () => {
    const t = draft.trim();
    if (t) api.setTitle(t);
    setRenaming(false);
  };

  const items: ContextMenuEntry[] = [
    { kind: 'item', label: 'Rename tab', onSelect: startRename },
    ...(locked
      ? []
      : ([
          { kind: 'separator' },
          { kind: 'item', label: 'Close', danger: true, onSelect: () => api.close() },
        ] as ContextMenuEntry[])),
  ];

  return (
    <ContextMenu
      items={items}
      trigger={
        <span
          className={styles.tab}
          onMouseDown={(e) => {
            if (!locked && e.button === 1) {
              e.preventDefault();
              api.close();
            }
          }}
        >
          {icon && <Icon name={icon} size={13} className={styles.icon} />}
          {renaming ? (
            <input
              className={styles.rename}
              autoFocus
              value={draft}
              onChange={(e) => setDraft(e.currentTarget.value)}
              onBlur={commitRename}
              onKeyDown={(e) => {
                if (e.key === 'Enter') commitRename();
                else if (e.key === 'Escape') setRenaming(false);
              }}
              onPointerDown={(e) => e.stopPropagation()}
              onClick={(e) => e.stopPropagation()}
            />
          ) : (
            <span className={styles.title} onDoubleClick={startRename}>
              {api.title}
            </span>
          )}
          {showDot && (
            <span
              className={`${styles.dot} ${level === 'mention' ? styles.dotMention : ''}`}
              aria-label={level === 'mention' ? 'New mention' : 'New activity'}
            />
          )}
          {!locked && (
            <button
              type="button"
              className={styles.close}
              aria-label="Close tab"
              onPointerDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation();
                api.close();
              }}
            >
              ×
            </button>
          )}
        </span>
      }
    />
  );
}
