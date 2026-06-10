import type { IconName } from '../Icon';
import { listPlugins } from '../daemon/plugins';
import PluginPanel from './PluginPanel';
import { registerPanelContribution } from './registry';

// Manifest icons map onto the host icon set; anything unknown falls back to the plug.
const KNOWN_ICONS: IconName[] = ['chat', 'stream', 'stats', 'plugins', 'mentions', 'filter', 'gift', 'search', 'list', 'grid', 'star', 'zap'];

function iconFor(name?: string): IconName {
  return (KNOWN_ICONS as string[]).includes(name ?? '') ? (name as IconName) : 'plugins';
}

/**
 * Registers panel contributions for every enabled remote GUI plugin. Called at app start and
 * after plugin install/enable so new panels appear in the catalog without a reload.
 */
export async function syncPluginPanels(): Promise<void> {
  let plugins;
  try {
    plugins = await listPlugins();
  } catch {
    return; // daemon unreachable — nothing to register
  }
  for (const p of plugins) {
    if (p.built_in || p.state !== 'enabled' || !p.has_gui) continue;
    for (const panel of p.panels ?? []) {
      registerPanelContribution({
        kind: panel.kind,
        title: panel.title,
        icon: iconFor(panel.icon),
        render: () => <PluginPanel pluginId={p.id} />,
      });
    }
  }
}
