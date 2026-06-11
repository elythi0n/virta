import type { ReactNode } from 'react';
import type { IconName } from '../Icon';
import AskPanel from './AskPanel';
import CelebrationsPane from './CelebrationsPane';
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
export interface PanelContribution {
  kind: string;
  title: string;
  icon: IconName;
  render: (props: PanelRenderProps) => ReactNode;
  /** Listed in the Panels sidebar catalog. Default true; programmatic-only kinds set false. */
  catalog?: boolean;
}

// The registry of built-in panels. Order here is the order in the Panels catalog.
export const PANELS: PanelContribution[] = [
  { kind: 'feed', title: 'Chat', icon: 'chat', render: (p) => <FeedPanel channels={p.channels} panelId={p.panelId} /> },
  { kind: 'mentions', title: 'Mentions', icon: 'mentions', render: (p) => <MentionInbox panelId={p.panelId} /> },
  { kind: 'celebrations', title: 'Celebrations', icon: 'gift', render: (p) => <CelebrationsPane panelId={p.panelId} /> },
  { kind: 'highlights', title: 'Highlights', icon: 'flame', render: () => <HighlightsPanel /> },
  { kind: 'filters', title: 'Filters', icon: 'filter', render: () => <FiltersPanel /> },
  { kind: 'stream', title: 'Streams', icon: 'stream', render: () => <StreamPane /> },
  { kind: 'mods', title: 'Mod queue', icon: 'mods', render: () => <HeldQueuePanel /> },
  { kind: 'ask', title: 'Ask AI', icon: 'chat', render: () => <AskPanel /> },
  { kind: 'search', title: 'Search', icon: 'search', render: () => <SearchPanel /> },
  { kind: 'stats', title: 'Stats', icon: 'stats', render: () => <StatsPanel /> },
  { kind: 'markets', title: 'Markets', icon: 'stats', render: () => <MarketsPanel /> },
  { kind: 'obs', title: 'OBS', icon: 'stream', render: () => <OBSPanel /> },
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

export function subscribePanelCatalog(cb: () => void): () => void {
  catalogListeners.add(cb);
  return () => catalogListeners.delete(cb);
}

export function panelCatalogVersion(): number {
  return catalogVersion;
}
