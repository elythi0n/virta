import type { IconName } from '../Icon';

// The primary activity-bar views, which toggle the side bar. Settings is not one of them: the
// activity-bar gear opens Settings as a full dock panel (it has too much IA for a 264px rail).
export type ViewId = 'sources' | 'panels' | 'streams';

export interface ViewDef {
  id: ViewId;
  label: string;
  icon: IconName;
}

export const PRIMARY_VIEWS: ViewDef[] = [
  { id: 'panels', label: 'Panels', icon: 'panels' },
  { id: 'streams', label: 'Streams', icon: 'stream' },
  { id: 'sources', label: 'Sources', icon: 'sources' },
];

// Catalog of panels the user can open into the dock from the Panels view.
export interface PanelDef {
  kind: string;
  title: string;
  icon: IconName;
}

export const PANEL_CATALOG: PanelDef[] = [
  { kind: 'feed', title: 'Unified feed', icon: 'chat' },
  { kind: 'mentions', title: 'Mentions', icon: 'mentions' },
  { kind: 'celebrations', title: 'Celebrations', icon: 'gift' },
  { kind: 'filters', title: 'Filters', icon: 'filter' },
  { kind: 'x-chat', title: 'X chat', icon: 'x' },
  { kind: 'stream', title: 'Stream', icon: 'stream' },
  { kind: 'mods', title: 'Mod queue', icon: 'mods' },
  { kind: 'stats', title: 'Stats', icon: 'stats' },
];

// Platforms shown in the Sources view. Connection wiring lands when the daemon client is added.
export interface SourceDef {
  id: 'twitch' | 'kick' | 'x';
  label: string;
  accent: string;
}

export const SOURCES: SourceDef[] = [
  { id: 'twitch', label: 'Twitch', accent: 'var(--virta-plat-twitch)' },
  { id: 'kick', label: 'Kick', accent: 'var(--virta-plat-kick)' },
  { id: 'x', label: 'X', accent: 'var(--virta-plat-x)' },
];
