import { useState } from 'react';
import { Button, Text, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { PANEL_CATALOG, type ViewId } from './views';
import AddChannel from './AddChannel';
import StreamsSidebar, { type StreamLayout } from './StreamsSidebar';
import styles from './SideBar.module.css';

const LAYOUT_KEY = 'virta.streams.layout';
function loadLayout(): StreamLayout {
  try {
    return localStorage.getItem(LAYOUT_KEY) === 'list' ? 'list' : 'cards';
  } catch {
    return 'cards';
  }
}

type Props = {
  view: ViewId;
  openPanel: (kind: string, title: string) => void;
  openChannel: (channelKey: string, label: string) => void;
  openStream: (channelKey: string, label: string) => void;
  listFeeds: () => { id: string; title: string }[];
  mergeChannelIntoFeed: (panelId: string, channelKey: string) => void;
  onNewFeed: () => void;
  /** When true the sidebar is visually hidden but stays mounted so its hooks keep running. */
  hidden?: boolean;
};

const TITLES: Record<ViewId, string> = {
  panels: 'Panels',
  streams: 'Streams',
};

export default function SideBar({ view, openPanel, openChannel, openStream, listFeeds, mergeChannelIntoFeed, onNewFeed, hidden }: Props) {
  const [layout, setLayout] = useState<StreamLayout>(loadLayout);
  const toggleLayout = () =>
    setLayout((l) => {
      const next: StreamLayout = l === 'cards' ? 'list' : 'cards';
      try {
        localStorage.setItem(LAYOUT_KEY, next);
      } catch {
        // storage unavailable; the choice just won't persist across reloads
      }
      return next;
    });

  return (
    <aside className={styles.side} aria-label={TITLES[view]} hidden={hidden} style={hidden ? { display: 'none' } : undefined}>
      <header className={styles.head}>
        <Text variant="meta" tone="subtle">
          {TITLES[view]}
        </Text>
        {view === 'streams' && (
          <div className={styles.headActions}>
            <Tooltip content={layout === 'cards' ? 'List view' : 'Thumbnail view'} side="bottom">
              <button type="button" className={styles.layoutToggle} aria-label="Toggle stream layout" onClick={toggleLayout}>
                <Icon name={layout === 'cards' ? 'list' : 'grid'} size={18} />
              </button>
            </Tooltip>
            <AddChannel />
          </div>
        )}
      </header>
      <div className={styles.body}>
        {view === 'panels' && (
          <>
            <ul className={styles.rows}>
              {PANEL_CATALOG.map((p) => (
                <li key={p.kind}>
                  <Button variant="ghost" size="sm" className={styles.rowBtn} onClick={() => openPanel(p.kind, p.title)}>
                    <span className={styles.glyph}>
                      <Icon name={p.icon} size={16} />
                    </span>
                    <Text variant="ui" tone="inherit" className={styles.rowLabel}>
                      {p.title}
                    </Text>
                  </Button>
                </li>
              ))}
            </ul>
            <Button variant="subtle" size="sm" className={styles.newFeed} onClick={onNewFeed}>
              + New feed
            </Button>
          </>
        )}

        {view === 'streams' && (
          <StreamsSidebar
            layout={layout}
            openChannel={openChannel}
            openStream={openStream}
            listFeeds={listFeeds}
            mergeChannelIntoFeed={mergeChannelIntoFeed}
          />
        )}
      </div>
    </aside>
  );
}
