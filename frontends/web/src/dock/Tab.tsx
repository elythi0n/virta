import { useState } from 'react';
import type { IDockviewPanelHeaderProps } from 'dockview';
import { ContextMenu, type ContextMenuEntry } from '@virta/ui-kit';
import NewFeedDialog from '../shell/NewFeedDialog';
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
          <span className={styles.tab}>
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
