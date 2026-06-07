import { useState } from 'react';
import { Badge, Text } from '@virta/ui-kit';
import Icon, { type IconName } from '../Icon';
import styles from './PluginsPanel.module.css';

type PluginStatus = 'installed' | 'available' | 'coming-soon';

interface PluginDef {
  id: string;
  name: string;
  description: string;
  author: string;
  icon: IconName;
  status: PluginStatus;
  tags: string[];
}

const PLUGINS: PluginDef[] = [
  { id: 'virta.chat',         name: 'Chat',          description: 'Unified live chat across Twitch, Kick, and X in one feed.',                       author: 'Virta', icon: 'chat',     status: 'installed',   tags: ['chat', 'feed'] },
  { id: 'virta.mentions',     name: 'Mentions',      description: 'Collects every message that names you across all channels.',                        author: 'Virta', icon: 'mentions', status: 'installed',   tags: ['chat'] },
  { id: 'virta.celebrations', name: 'Celebrations',  description: 'Shoutout wall for subs, raids, gifts, and announcements.',                          author: 'Virta', icon: 'gift',     status: 'installed',   tags: ['events'] },
  { id: 'virta.filters',      name: 'Filters',       description: 'Highlight, hide, or mask messages by keyword, author, regex, or platform.',         author: 'Virta', icon: 'filter',   status: 'installed',   tags: ['moderation'] },
  { id: 'virta.mods',         name: 'Mod queue',     description: 'AutoMod hold queue — approve or deny held messages from the dock.',                 author: 'Virta', icon: 'mods',     status: 'installed',   tags: ['moderation'] },
  { id: 'virta.search',       name: 'Search',        description: 'Full-text search over your chat history with channel and author filters.',          author: 'Virta', icon: 'search',   status: 'installed',   tags: ['history'] },
  { id: 'virta.ask-ai',       name: 'Ask AI',        description: 'Ask questions about chat history using an AI agent: top fans, user stats.',         author: 'Virta', icon: 'chat',     status: 'installed',   tags: ['ai'] },
  { id: 'virta.stream',       name: 'Streams',       description: 'Grid of all joined channels with live thumbnails, viewer counts, and quick launch.', author: 'Virta', icon: 'stream',   status: 'installed',   tags: ['stream'] },
  { id: 'virta.stats',        name: 'Stats',         description: 'Live stats: messages per second, unique chatters, top emotes, activity timelines.', author: 'Virta', icon: 'stats',    status: 'coming-soon', tags: ['analytics'] },
  { id: 'virta.markets',      name: 'Markets',       description: 'Real-time ticker for crypto, stocks, and FX via exchange WebSockets.',              author: 'Virta', icon: 'stats',    status: 'coming-soon', tags: ['data'] },
];

type FilterTab = 'all' | 'installed' | 'available';

const STATUS_LABEL: Record<PluginStatus, string> = {
  installed: 'Installed',
  available: 'Available',
  'coming-soon': 'Soon',
};
const STATUS_TONE: Record<PluginStatus, 'ok' | 'neutral' | 'warn'> = {
  installed: 'ok',
  available: 'neutral',
  'coming-soon': 'warn',
};

export default function PluginsPanel() {
  const [filter, setFilter] = useState<FilterTab>('all');

  const counts: Record<FilterTab, number> = {
    all: PLUGINS.length,
    installed: PLUGINS.filter(p => p.status === 'installed').length,
    available: PLUGINS.filter(p => p.status === 'available').length,
  };

  const visible = PLUGINS.filter(p => {
    if (filter === 'installed') return p.status === 'installed';
    if (filter === 'available') return p.status === 'available';
    return true;
  });

  return (
    <div className={styles.panel}>
      <header className={styles.head}>
        <div className={styles.headIcon}><Icon name="plugins" size={16} /></div>
        <div className={styles.headText}>
          <Text variant="title" as="h2" className={styles.title}>Plugins</Text>
          <Text variant="meta" tone="subtle" as="p" className={styles.lead}>
            Extend Virta with panels, commands, and live data. Third-party installs planned for a future release.
          </Text>
        </div>
      </header>

      <div className={styles.filterBar} role="tablist" aria-label="Filter plugins">
        {(['all', 'installed', 'available'] as FilterTab[]).map(tab => (
          <button
            key={tab}
            type="button"
            role="tab"
            aria-selected={filter === tab}
            className={`${styles.tab} ${filter === tab ? styles.tabOn : ''}`}
            onClick={() => setFilter(tab)}
          >
            {tab === 'all' ? 'All' : tab === 'installed' ? 'Installed' : 'Available'}
            <span className={styles.tabCount}>{counts[tab]}</span>
          </button>
        ))}
      </div>

      <ul className={styles.grid} role="list">
        {visible.map(p => (
          <li key={p.id} className={styles.item}>
            <div
              className={[
                styles.card,
                p.status === 'installed' ? styles.installed : '',
                p.status === 'coming-soon' ? styles.soon : '',
              ].filter(Boolean).join(' ')}
            >
              <div className={styles.cardTop}>
                <span className={styles.cardIcon}>
                  <Icon name={p.icon} size={16} />
                </span>
                <Badge tone={STATUS_TONE[p.status]}>{STATUS_LABEL[p.status]}</Badge>
              </div>

              <div className={styles.cardName}>{p.name}</div>

              <p className={styles.cardDesc}>{p.description}</p>

              <div className={styles.cardFooter}>
                <span className={styles.cardAuthor}>{p.author}</span>
                <div className={styles.cardTags}>
                  {p.tags.map(t => <span key={t} className={styles.tag}>{t}</span>)}
                </div>
              </div>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
