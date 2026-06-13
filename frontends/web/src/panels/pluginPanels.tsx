import type { IconName } from '../Icon';
import { listPlugins } from '../daemon/plugins';
import PluginPanel from './PluginPanel';
import { registerPanelContribution, removePanelContribution } from './registry';

// Manifest icons map onto the host icon set; anything unknown falls back to the plug.
const KNOWN_ICONS: IconName[] = ['chat', 'stream', 'stats', 'plugins', 'mentions', 'filter', 'gift', 'search', 'list', 'grid', 'star', 'zap'];

function iconFor(name?: string): IconName {
  return (KNOWN_ICONS as string[]).includes(name ?? '') ? (name as IconName) : 'plugins';
}

// Tracks which panel kinds were contributed by remote plugins so we can remove them on uninstall/disable.
const pluginPanelKinds = new Set<string>();

/**
 * Registers panel contributions for every enabled remote GUI plugin, and removes contributions
 * for plugins that are no longer enabled. Called at app start and after any plugin state change.
 */
export async function syncPluginPanels(): Promise<void> {
  let plugins;
  try {
    plugins = await listPlugins();
  } catch {
    return; // daemon unreachable — nothing to register
  }

  const activeKinds = new Set<string>();
  for (const p of plugins) {
    if (p.built_in || p.state !== 'enabled' || !p.has_gui) continue;
    for (const panel of p.panels ?? []) {
      activeKinds.add(panel.kind);
      pluginPanelKinds.add(panel.kind);
      registerPanelContribution({
        kind: panel.kind,
        title: panel.title,
        icon: iconFor(panel.icon),
        render: () => <PluginPanel pluginId={p.id} />,
      });
    }
  }

  // Remove sidebar entries for plugins that are no longer enabled or installed.
  for (const kind of [...pluginPanelKinds]) {
    if (!activeKinds.has(kind)) {
      removePanelContribution(kind);
      pluginPanelKinds.delete(kind);
    }
  }
}
