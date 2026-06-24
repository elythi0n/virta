import type { ReactNode } from 'react';
import type { IconName } from '../Icon';
import AskPanel from './AskPanel';
import CelebrationsPane from './CelebrationsPane';
import DiscoveryPanel from './DiscoveryPanel';
import EncoderHealthPane from './EncoderHealthPane';
import OBSControlPane from './OBSControlPane';
import OBSPanel from './OBSPanel';
import FeedPanel from './FeedPanel';
import FiltersPanel from './FiltersPanel';
import HeldQueuePanel from './HeldQueuePanel';
import HighlightsPanel from './HighlightsPanel';
import MentionInbox from './MentionInbox';
import MarketsPanel from './MarketsPanel';
import PluginsPanel from './PluginsPanel';
import SearchPanel from './SearchPanel';
import StatsPanel from './StatsPanel';
import StreamPane from './StreamPane';
import WatchPane from './WatchPane';

export interface PanelRenderProps {
  channels?: string[];
  panelId?: string;
}

// A panel contribution: how the dock renders a kind, and whether it lists in the Panels catalog.
// Our own panels register here exactly the way a third-party plugin's panel will — the dock opens a
// panel by looking its kind up in this registry, never a hardcoded switch. A plugin host later
// just appends contributions to this same list.
// Section a contribution appears under in the "+" popover. Order is meaningful: groups render in
// PANEL_GROUPS order; anything missing falls into "Plugins" at the bottom so user-installed
// panels naturally surface together.
export type PanelGroup = 'Chat' | 'Stream' | 'Broadcast' | 'Tools' | 'Plugins';
export const PANEL_GROUPS: PanelGroup[] = ['Chat', 'Stream', 'Broadcast', 'Tools', 'Plugins'];

export interface PanelContribution {
  kind: string;
  title: string;
  icon: IconName;
  render: (props: PanelRenderProps) => ReactNode;
  /** Listed in the Panels sidebar catalog. Default true; programmatic-only kinds set false. */
  catalog?: boolean;
  /** Section header in the "+" popover. Defaults to "Plugins" when omitted. */
  group?: PanelGroup;
}

// The registry of built-in panels. Order inside a group is the order shown in the section.
export const PANELS: PanelContribution[] = [
  // Chat — reading, replying, moderating, searching messages.
  { kind: 'feed', group: 'Chat', title: 'Chat', icon: 'chat', render: (p) => <FeedPanel channels={p.channels} panelId={p.panelId} /> },
  { kind: 'mentions', group: 'Chat', title: 'Mentions', icon: 'mentions', render: (p) => <MentionInbox panelId={p.panelId} /> },
  { kind: 'mods', group: 'Chat', title: 'Mod queue', icon: 'mods', render: () => <HeldQueuePanel /> },
  { kind: 'highlights', group: 'Chat', title: 'Highlights', icon: 'flame', render: () => <HighlightsPanel /> },
  { kind: 'filters', group: 'Chat', title: 'Filters', icon: 'filter', render: () => <FiltersPanel /> },
  { kind: 'search', group: 'Chat', title: 'Search', icon: 'search', render: () => <SearchPanel /> },
  // Stream — what the live channels are doing right now.
  { kind: 'stream', group: 'Stream', title: 'Streams', icon: 'stream', render: () => <StreamPane /> },
  { kind: 'discovery', group: 'Stream', title: 'Discovery', icon: 'search', render: () => <DiscoveryPanel /> },
  { kind: 'stats', group: 'Stream', title: 'Stats', icon: 'stats', render: () => <StatsPanel /> },
  { kind: 'celebrations', group: 'Stream', title: 'Gifts', icon: 'gift', render: (p) => <CelebrationsPane panelId={p.panelId} /> },
  // Broadcast — owning the broadcast: OBS, encoder health, configuration.
  { kind: 'obs-controls', group: 'Broadcast', title: 'OBS controls', icon: 'sliders', render: () => <OBSControlPane /> },
  { kind: 'encoder-health', group: 'Broadcast', title: 'Encoder health', icon: 'gauge', render: () => <EncoderHealthPane /> },
  { kind: 'obs', group: 'Broadcast', title: 'OBS settings', icon: 'stream', render: () => <OBSPanel /> },
  // Tools — adjacent utilities that aren't chat or stream surfaces.
  { kind: 'ask', group: 'Tools', title: 'Ask AI', icon: 'chat', render: () => <AskPanel /> },
  { kind: 'markets', group: 'Tools', title: 'Markets', icon: 'stats', render: () => <MarketsPanel /> },
  // Opened programmatically, not from the catalog.
  { kind: 'watch', title: 'Stream', icon: 'stream', catalog: false, render: (p) => <WatchPane channel={p.channels?.[0]} /> },
  { kind: 'plugins', title: 'Plugins', icon: 'plugins', catalog: false, render: () => <PluginsPanel /> },
];

// The catalog is just the contributions that opt into being listed — derived, never a second list.
export const PANEL_CATALOG = PANELS.filter((p) => p.catalog !== false);

export function panelByKind(kind: string): PanelContribution | undefined {
  return PANELS.find((p) => p.kind === kind);
}

// ── Runtime contributions (plugins) ─────────────────────────────────────────
// Remote plugins append here after the daemon reports them. PANELS/PANEL_CATALOG are mutated in
// place (consumers .map at render time); the version store lets catalog surfaces re-render.
let catalogVersion = 0;
const catalogListeners = new Set<() => void>();

export function registerPanelContribution(c: PanelContribution): void {
  const existing = PANELS.findIndex((p) => p.kind === c.kind);
  if (existing >= 0) {
    PANELS[existing] = c; // re-sync (e.g. plugin reinstalled with a new title)
    const ci = PANEL_CATALOG.findIndex((p) => p.kind === c.kind);
    if (ci >= 0 && c.catalog !== false) PANEL_CATALOG[ci] = c;
  } else {
    PANELS.push(c);
    if (c.catalog !== false) PANEL_CATALOG.push(c);
  }
  catalogVersion += 1;
  catalogListeners.forEach((l) => l());
}

export function removePanelContribution(kind: string): void {
  const pi = PANELS.findIndex((p) => p.kind === kind);
  if (pi >= 0) PANELS.splice(pi, 1);
  const ci = PANEL_CATALOG.findIndex((p) => p.kind === kind);
  if (ci >= 0) PANEL_CATALOG.splice(ci, 1);
  catalogVersion += 1;
  catalogListeners.forEach((l) => l());
}

export function subscribePanelCatalog(cb: () => void): () => void {
  catalogListeners.add(cb);
  return () => catalogListeners.delete(cb);
}

export function panelCatalogVersion(): number {
  return catalogVersion;
}
