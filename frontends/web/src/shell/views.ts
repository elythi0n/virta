import type { IconName } from '../Icon.svelte';

// The primary activity-bar views. `settings` lives in the footer group, the rest at the top.
export type ViewId = 'sources' | 'panels' | 'settings';

export interface ViewDef {
  id: ViewId;
  label: string;
  icon: IconName;
}

export const PRIMARY_VIEWS: ViewDef[] = [
  { id: 'panels', label: 'Panels', icon: 'panels' },
  { id: 'sources', label: 'Sources', icon: 'sources' },
];

export const FOOTER_VIEWS: ViewDef[] = [{ id: 'settings', label: 'Settings', icon: 'settings' }];

// Catalog of panels the user can open into the dock from the Panels view.
export interface PanelDef {
  kind: string;
  title: string;
  icon: IconName;
}

export const PANEL_CATALOG: PanelDef[] = [
  { kind: 'feed', title: 'Unified feed', icon: 'chat' },
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
