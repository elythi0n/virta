import { Button, Text } from '@virta/ui-kit';
import Icon from '../Icon';
import { PANEL_CATALOG, type ViewId } from './views';
import AddChannel from './AddChannel';
import StreamsSidebar from './StreamsSidebar';
import styles from './SideBar.module.css';

type Props = {
  view: ViewId;
  openPanel: (kind: string, title: string) => void;
  openChannel: (channelKey: string, label: string) => void;
  openStream: (channelKey: string, label: string) => void;
  listFeeds: () => { id: string; title: string }[];
  mergeChannelIntoFeed: (panelId: string, channelKey: string) => void;
  onNewFeed: () => void;
};

const TITLES: Record<ViewId, string> = {
  panels: 'Panels',
  streams: 'Streams',
};

export default function SideBar({ view, openPanel, openChannel, openStream, listFeeds, mergeChannelIntoFeed, onNewFeed }: Props) {
  return (
    <aside className={styles.side} aria-label={TITLES[view]}>
      <header className={styles.head}>
        <Text variant="meta" tone="subtle">
          {TITLES[view]}
        </Text>
        {view === 'streams' && <AddChannel />}
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
