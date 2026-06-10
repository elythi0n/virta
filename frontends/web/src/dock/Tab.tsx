import { useEffect, useState } from 'react';
import type { IDockviewPanelHeaderProps } from 'dockview';
import { ContextMenu, type ContextMenuEntry } from '@virta/ui-kit';
import Icon, { type IconName } from '../Icon';
import NewFeedDialog from '../shell/NewFeedDialog';
import { panelByKind } from '../panels/registry';
import { activityLevel, clearActivity, useActivityVersion } from './activity';
import styles from './Tab.module.css';

// Custom dockview tab: the title + a close affordance, plus a right-click menu (rename inline, and
// for feed tabs "Customize feed…"). We render our own chrome so the menu and rename live on the
// tab itself; dockview's tab container still provides active/hover styling and drag.
export default function Tab(props: IDockviewPanelHeaderProps) {
  const { api, params } = props;
  const [renaming, setRenaming] = useState(false);
  const [draft, setDraft] = useState('');
  const [editFeed, setEditFeed] = useState(false);

  const isFeed = params.kind === 'feed';
  const channels = Array.isArray(params.channels) ? (params.channels as string[]) : [];

  // Activity dot: shown while the panel has unseen content and is a background tab. Any
  // activation change clears the mark, so the dot only reflects arrivals since you left.
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

  // The leading glyph comes from the panel registry's contribution for this kind; Settings is the
  // one panel registered outside it.
  const kind = typeof params.kind === 'string' ? params.kind : undefined;
  const icon: IconName | undefined = kind ? panelByKind(kind)?.icon : api.id === 'settings' ? 'settings' : undefined;

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
    ...(isFeed ? [{ kind: 'item' as const, label: 'Customize feed…', onSelect: () => setEditFeed(true) }] : []),
    { kind: 'separator' },
    { kind: 'item', label: 'Close', danger: true, onSelect: () => api.close() },
  ];

  return (
    <>
      <ContextMenu
        items={items}
        trigger={
          <span
            className={styles.tab}
            onMouseDown={(e) => {
              // Middle-click closes, matching browser and editor tab strips.
              if (e.button === 1) {
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
          </span>
        }
      />
      {isFeed && (
        <NewFeedDialog
          open={editFeed}
          onClose={() => setEditFeed(false)}
          dialogTitle="Customize feed"
          submitLabel="Save"
          initialName={api.title}
          initialChannels={channels}
          onSubmit={(title, next) => {
            api.updateParameters({ kind: 'feed', channels: next, title });
            api.setTitle(title);
          }}
        />
      )}
    </>
  );
}
