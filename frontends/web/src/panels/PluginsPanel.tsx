import { useCallback, useEffect, useState } from 'react';
import { Badge, Button, Dialog, Input, Text } from '@virta/ui-kit';
import type { IconName } from '../Icon';
import Icon from '../Icon';
import { listPlugins, getPlugin, enablePlugin, disablePlugin, installPlugin, uninstallPlugin, putPluginConfig } from '../daemon/plugins';
import type { PluginInfo, PluginDetail } from '../daemon/plugins';
import { syncPluginPanels } from './pluginPanels';
import SchemaForm from './SchemaForm';
import styles from './PluginsPanel.module.css';

type FilterTab = 'all' | 'installed' | 'available';

// Static catalog used as fallback while the API is loading.
const BUILT_IN_CATALOG: PluginInfo[] = [
  { id: 'virta.chat',         name: 'Chat',          description: 'Unified live chat across Twitch, Kick, and X in one feed.',                         built_in: true, state: 'enabled',  tags: ['chat', 'feed'],   version: '1.0.0', publisher: 'Virta' },
  { id: 'virta.mentions',     name: 'Mentions',      description: 'Collects every message that names you across all channels.',                         built_in: true, state: 'enabled',  tags: ['chat'],           version: '1.0.0', publisher: 'Virta' },
  { id: 'virta.celebrations', name: 'Celebrations',  description: 'Shoutout wall for subs, raids, gifts, and announcements.',                           built_in: true, state: 'enabled',  tags: ['events'],         version: '1.0.0', publisher: 'Virta' },
  { id: 'virta.filters',      name: 'Filters',       description: 'Highlight, hide, or mask messages by keyword, author, regex, or platform.',          built_in: true, state: 'enabled',  tags: ['moderation'],     version: '1.0.0', publisher: 'Virta' },
  { id: 'virta.mods',         name: 'Mod queue',     description: 'AutoMod hold queue — approve or deny held messages from the dock.',                  built_in: true, state: 'enabled',  tags: ['moderation'],     version: '1.0.0', publisher: 'Virta' },
  { id: 'virta.search',       name: 'Search',        description: 'Full-text search over your chat history with channel and author filters.',            built_in: true, state: 'enabled',  tags: ['history'],        version: '1.0.0', publisher: 'Virta' },
  { id: 'virta.ask-ai',       name: 'Ask AI',        description: 'Ask questions about chat history using an AI agent: top fans, user stats.',          built_in: true, state: 'enabled',  tags: ['ai'],             version: '1.0.0', publisher: 'Virta' },
  { id: 'virta.stream',       name: 'Streams',       description: 'Grid of all joined channels with live thumbnails, viewer counts, and quick launch.', built_in: true, state: 'enabled',  tags: ['stream'],         version: '1.0.0', publisher: 'Virta' },
  { id: 'com.virta.markets',  name: 'Markets',       description: 'Real-time crypto price ticker and board via free exchange WebSockets.',               built_in: true, state: 'enabled',  tags: ['data', 'crypto'], version: '1.0.0', publisher: 'Virta', has_config: true },
  { id: 'virta.stats',        name: 'Stats',         description: 'Live stats: messages per second, unique chatters, top emotes, activity timelines.',  built_in: true, state: 'disabled', tags: ['analytics'],      version: '0.0.0', publisher: 'Virta' },
];

const ICON_BY_ID: Record<string, IconName> = {
  'virta.chat': 'chat', 'virta.mentions': 'mentions', 'virta.celebrations': 'gift',
  'virta.filters': 'filter', 'virta.mods': 'mods', 'virta.search': 'search',
  'virta.ask-ai': 'chat', 'virta.stream': 'stream', 'com.virta.markets': 'stats',
  'virta.stats': 'stats',
};
const pluginIcon = (id: string): IconName => ICON_BY_ID[id] ?? 'plugins';

export default function PluginsPanel() {
  const [filter, setFilter] = useState<FilterTab>('all');
  const [plugins, setPlugins] = useState<PluginInfo[]>(BUILT_IN_CATALOG);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [installUrl, setInstallUrl] = useState('');
  const [installing, setInstalling] = useState(false);
  const [installErr, setInstallErr] = useState('');
  // Plugin settings dialog
  const [settingsPlugin, setSettingsPlugin] = useState<PluginDetail | null>(null);
  const [settingsValues, setSettingsValues] = useState<Record<string, unknown>>({});

  const reload = useCallback(() => {
    listPlugins().then(live => {
      const byId = new Map<string, PluginInfo>();
      BUILT_IN_CATALOG.forEach(p => byId.set(p.id, p));
      live.forEach(p => byId.set(p.id, p));
      setPlugins([...byId.values()]);
    }).catch(() => {});
    // Newly installed/enabled GUI plugins contribute panels — keep the catalog in sync.
    void syncPluginPanels();
  }, []);

  useEffect(() => { reload(); }, [reload]);

  const toggle = useCallback(async (p: PluginInfo) => {
    setBusy(b => ({ ...b, [p.id]: true }));
    try {
      await (p.state === 'enabled' ? disablePlugin(p.id) : enablePlugin(p.id));
      reload();
    } catch { /* keep state */ } finally {
      setBusy(b => ({ ...b, [p.id]: false }));
    }
  }, [reload]);

  const doInstall = useCallback(async () => {
    const url = installUrl.trim();
    if (!url) return;
    setInstalling(true); setInstallErr('');
    try {
      await installPlugin(url);
      setInstallUrl('');
      reload();
    } catch (e) {
      setInstallErr(e instanceof Error ? e.message : 'Installation failed');
    } finally {
      setInstalling(false);
    }
  }, [installUrl, reload]);

  const doUninstall = useCallback(async (id: string) => {
    setBusy(b => ({ ...b, [id]: true }));
    try { await uninstallPlugin(id); reload(); }
    finally { setBusy(b => ({ ...b, [id]: false })); }
  }, [reload]);

  const openSettings = useCallback(async (id: string) => {
    try {
      const detail = await getPlugin(id);
      setSettingsPlugin(detail);
      setSettingsValues(detail.config ?? {});
    } catch { /* plugin may not have settings */ }
  }, []);

  const [savingSettings, setSavingSettings] = useState(false);
  const saveSettings = useCallback(async () => {
    if (!settingsPlugin) return;
    setSavingSettings(true);
    try {
      await putPluginConfig(settingsPlugin.id, settingsValues);
      setSettingsPlugin(null);
    } catch { /* keep the dialog open so nothing is silently lost */ }
    finally { setSavingSettings(false); }
  }, [settingsPlugin, settingsValues]);

  const counts = {
    all: plugins.length,
    installed: plugins.filter(p => p.state === 'enabled').length,
    available: plugins.filter(p => p.state === 'disabled').length,
  };

  const visible = plugins.filter(p =>
    filter === 'installed' ? p.state === 'enabled' :
    filter === 'available' ? p.state === 'disabled' : true,
  );

  return (
    <div className={styles.panel}>
      <header className={styles.head}>
        <div className={styles.headIcon}><Icon name="plugins" size={16} /></div>
        <div className={styles.headText}>
          <Text variant="title" as="h2" className={styles.title}>Plugins</Text>
          <Text variant="meta" tone="subtle" as="p" className={styles.lead}>
            Extend Virta with panels, commands, and live data.
          </Text>
        </div>
      </header>

      <div className={styles.installBar}>
        <Input
          aria-label="Plugin Git URL"
          placeholder="https://github.com/user/virta-plugin  (install from a Git URL)"
          value={installUrl}
          onChange={e => setInstallUrl(e.currentTarget.value)}
          onKeyDown={e => e.key === 'Enter' && void doInstall()}
        />
        <Button variant="solid" size="sm" disabled={!installUrl.trim() || installing} onClick={() => void doInstall()}>
          {installing ? 'Installing…' : 'Install'}
        </Button>
        {installErr && <p className={styles.installErr}>{installErr}</p>}
      </div>

      <div className={styles.filterBar} role="tablist" aria-label="Filter plugins">
        {(['all', 'installed', 'available'] as FilterTab[]).map(tab => (
          <button key={tab} type="button" role="tab" aria-selected={filter === tab}
            className={`${styles.tab} ${filter === tab ? styles.tabOn : ''}`}
            onClick={() => setFilter(tab)}>
            {tab === 'all' ? 'All' : tab === 'installed' ? 'Enabled' : 'Disabled'}
            <span className={styles.tabCount}>{counts[tab]}</span>
          </button>
        ))}
      </div>

      <ul className={styles.grid} role="list">
        {visible.map(p => {
          const isBusy = !!busy[p.id];
          const isEnabled = p.state === 'enabled';
          const isError = p.state === 'error';
          return (
            <li key={p.id} className={styles.item}>
              <div className={`${styles.card} ${isEnabled ? styles.installed : ''} ${isError ? styles.errored : ''}`}>
                <div className={styles.cardTop}>
                  <span className={styles.cardIcon}><Icon name={pluginIcon(p.id)} size={16} /></span>
                  <div className={styles.cardBadges}>
                    <Badge tone={isEnabled ? 'ok' : isError ? 'danger' : 'neutral'}>
                      {isEnabled ? 'Enabled' : isError ? 'Error' : 'Disabled'}
                    </Badge>
                    {p.built_in && <Badge tone="neutral">Built-in</Badge>}
                  </div>
                </div>
                <div className={styles.cardName}>{p.name}</div>
                <p className={styles.cardDesc}>{p.description}</p>
                {isError && p.error && <p className={styles.cardError}>{p.error}</p>}
                <div className={styles.cardFooter}>
                  <span className={styles.cardAuthor}>{p.publisher ?? 'Third-party'}</span>
                  <div className={styles.cardActions}>
                    {!p.built_in && isEnabled && (
                      <button type="button" className={styles.uninstallBtn}
                        disabled={isBusy} onClick={() => void doUninstall(p.id)}>
                        Uninstall
                      </button>
                    )}
                    {p.has_config && (
                      <button type="button" className={styles.settingsBtn}
                        aria-label={`Settings for ${p.name}`} title="Settings"
                        onClick={() => void openSettings(p.id)}>
                        <Icon name="settings" size={13} />
                      </button>
                    )}
                    <button type="button"
                      className={`${styles.toggleBtn} ${isEnabled ? styles.toggleBtnOn : ''}`}
                      disabled={isBusy}
                      onClick={() => void toggle(p)}>
                      {isBusy ? '…' : isEnabled ? 'Disable' : 'Enable'}
                    </button>
                  </div>
                </div>
                {(p.tags ?? []).length > 0 && (
                  <div className={styles.cardTags}>
                    {(p.tags ?? []).map(t => <span key={t} className={styles.tag}>{t}</span>)}
                  </div>
                )}
              </div>
            </li>
          );
        })}
      </ul>

      {/* Plugin settings dialog */}
      <Dialog
        open={settingsPlugin !== null}
        onOpenChange={o => !o && setSettingsPlugin(null)}
        title={settingsPlugin ? `${settingsPlugin.name} settings` : ''}
        description={settingsPlugin?.description}
        size="sm"
        footer={
          <>
            <Button variant="ghost" size="md" onClick={() => setSettingsPlugin(null)}>Close</Button>
            <Button variant="solid" size="md" disabled={savingSettings} onClick={() => void saveSettings()}>
              {savingSettings ? 'Saving…' : 'Save'}
            </Button>
          </>
        }
      >
        {settingsPlugin?.config_schema ? (
          <SchemaForm
            schema={settingsPlugin.config_schema as never}
            value={settingsValues}
            onChange={setSettingsValues}
          />
        ) : (
          <Text variant="body" tone="subtle">This plugin has no configurable settings.</Text>
        )}
      </Dialog>
    </div>
  );
}
