import type { IconName } from '../Icon';

// The primary activity-bar views, which toggle the side bar. Settings is not one of them: the
// activity-bar gear opens Settings as a full dock panel (it has too much IA for a 264px rail).
// Accounts live in Settings → Connections; channels are managed entirely from the Streams view
// (add via its +, leave via a stream's right-click), so there's no separate Sources view.
export type ViewId = 'panels' | 'streams';

export interface ViewDef {
  id: ViewId;
  label: string;
  icon: IconName;
}

export const PRIMARY_VIEWS: ViewDef[] = [
  { id: 'panels', label: 'Panels', icon: 'panels' },
  { id: 'streams', label: 'Streams', icon: 'stream' },
];

// Catalog of panels the user can open into the dock from the Panels view.
export interface PanelDef {
  kind: string;
  title: string;
  icon: IconName;
}

export const PANEL_CATALOG: PanelDef[] = [
  { kind: 'feed', title: 'Chat', icon: 'chat' },
  { kind: 'mentions', title: 'Mentions', icon: 'mentions' },
  { kind: 'celebrations', title: 'Celebrations', icon: 'gift' },
  { kind: 'filters', title: 'Filters', icon: 'filter' },
  { kind: 'x-chat', title: 'X chat', icon: 'x' },
  { kind: 'stream', title: 'Stream', icon: 'stream' },
  { kind: 'mods', title: 'Mod queue', icon: 'mods' },
  { kind: 'stats', title: 'Stats', icon: 'stats' },
];
