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

// The Panels catalog is the panel contribution registry's catalog-listed entries — one source of
// truth for both the sidebar list and how the dock renders each kind (panels/registry).
export { PANEL_CATALOG, type PanelContribution } from '../panels/registry';
