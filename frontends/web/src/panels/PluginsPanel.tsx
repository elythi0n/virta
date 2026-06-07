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
  { id: 'virta.chat', name: 'Chat', description: 'Unified live chat feed across Twitch, Kick, and X in one stream.', author: 'Virta', icon: 'chat', status: 'installed', tags: ['chat', 'feed'] },
  { id: 'virta.mentions', name: 'Mentions', description: 'Collects messages that name you across every channel so you never miss a shoutout.', author: 'Virta', icon: 'mentions', status: 'installed', tags: ['chat', 'alerts'] },
  { id: 'virta.celebrations', name: 'Celebrations', description: 'A calm shoutout wall for subs, raids, gifts, and announcements with subtle animations.', author: 'Virta', icon: 'gift', status: 'installed', tags: ['events', 'overlay'] },
  { id: 'virta.filters', name: 'Filters', description: 'Rule-based message filtering: highlight, hide, or mask by keyword, author, regex, or platform.', author: 'Virta', icon: 'filter', status: 'installed', tags: ['moderation', 'feed'] },
  { id: 'virta.mods', name: 'Mod queue', description: 'AutoMod held-message queue — approve or deny held messages directly from the dock.', author: 'Virta', icon: 'mods', status: 'installed', tags: ['moderation'] },
  { id: 'virta.search', name: 'Search', description: 'Full-text search over your logged chat history with channel and author filters.', author: 'Virta', icon: 'search', status: 'installed', tags: ['history', 'search'] },
  { id: 'virta.ask-ai', name: 'Ask AI', description: 'Ask questions about your chat history using an AI agent: top fans, user history, channel stats.', author: 'Virta', icon: 'chat', status: 'installed', tags: ['ai', 'history'] },
  { id: 'virta.stream', name: 'Stream viewer', description: 'Embedded stream player for Twitch and Kick channels, dockable alongside the chat.', author: 'Virta', icon: 'stream', status: 'installed', tags: ['stream'] },
  { id: 'virta.x-chat', name: 'X chat', description: 'Best-effort read of X (Twitter) broadcast chat via the x-bridge scraper. Labeled experimental.', author: 'Virta', icon: 'x', status: 'available', tags: ['chat', 'x'] },
  { id: 'virta.stats', name: 'Stats', description: 'Live channel stats: messages per second, unique chatters, top emotes, and activity timelines.', author: 'Virta', icon: 'stats', status: 'coming-soon', tags: ['analytics'] },
  { id: 'virta.markets', name: 'Markets', description: 'Real-time market ticker for crypto, stocks, and FX — live data via exchange WebSockets, no API key needed for crypto.', author: 'Virta', icon: 'stats', status: 'coming-soon', tags: ['data', 'ticker'] },
];

type FilterTab = 'all' | 'installed' | 'available';

const STATUS_LABEL: Record<PluginStatus, string> = { installed: 'Installed', available: 'Available', 'coming-soon': 'Coming soon' };
const STATUS_TONE: Record<PluginStatus, 'ok' | 'neutral' | 'warn'> = { installed: 'ok', available: 'neutral', 'coming-soon': 'warn' };

export default function PluginsPanel() {
  const [filter, setFilter] = useState<FilterTab>('all');

  // PLUGINS is a module-level constant — derive static counts once.
  const INSTALLED_COUNT = PLUGINS.filter(p => p.status === 'installed').length;
  // "Available" means plugins that are not installed but could be (not coming-soon).
  const AVAILABLE_COUNT = PLUGINS.filter(p => p.status === 'available').length;

  const visible = PLUGINS.filter(p => {
    if (filter === 'installed') return p.status === 'installed';
    // "available" tab shows installable plugins only, not coming-soon placeholders.
    if (filter === 'available') return p.status === 'available';
    return true;
  });

  const counts = {
    all: PLUGINS.length,
    installed: INSTALLED_COUNT,
    available: AVAILABLE_COUNT,
  };

  return (
    <div className={styles.panel}>
      <header className={styles.head}>
        <Icon name="plugins" size={20} />
        <div>
          <Text variant="title" as="h2" className={styles.title}>
            Plugins
          </Text>
          <Text variant="body" tone="subtle" as="p" className={styles.lead}>
            Plugins extend Virta with new panels, chat commands, and live data.
            Third-party install from a URL is planned for a future release.
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
            className={`${styles.filterTab} ${filter === tab ? styles.filterTabOn : ''}`}
            onClick={() => setFilter(tab)}
          >
            {tab === 'all' ? 'All' : tab === 'installed' ? 'Installed' : 'Available'}
            <span className={styles.filterCount}>{counts[tab]}</span>
          </button>
        ))}
      </div>

      <ul className={styles.list} role="list">
        {visible.map(p => (
          <li key={p.id}>
            <div className={`${styles.card} ${p.status === 'installed' ? styles.cardInstalled : ''}`}>
              {p.status === 'installed' && (
                <span className={styles.checkmark} aria-hidden="true">
                  <Icon name="check" size={11} />
                </span>
              )}
              <span className={styles.cardIcon}>
                <Icon name={p.icon} size={20} />
              </span>
              <div className={styles.cardBody}>
                <div className={styles.cardHead}>
                  <span className={styles.cardName}>{p.name}</span>
                  <span className={styles.cardAuthor}>{p.author}</span>
                  <Badge tone={STATUS_TONE[p.status]}>{STATUS_LABEL[p.status]}</Badge>
                </div>
                <Text variant="body" tone="subtle" as="p" className={styles.cardDesc}>
                  {p.description}
                </Text>
                <div className={styles.cardTags}>
                  {p.tags.map(t => (
                    <span key={t} className={styles.tag}>{t}</span>
                  ))}
                </div>
              </div>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
