import type { IconName } from '../Icon';

// The primary activity-bar views, which toggle the side bar. Settings is not one of them: the
// activity-bar gear opens Settings as a full dock panel (it has too much IA for a 264px rail).
// Accounts live in Settings → Connections; channels are managed entirely from the Streams view
// (add via its +, leave via a stream's right-click), so there's no separate Sources view.
export type ViewId = 'panels' | 'streams' | 'studio' | 'deck';

export interface ViewDef {
  id: ViewId;
  label: string;
  icon: IconName;
}

export const PRIMARY_VIEWS: ViewDef[] = [
  { id: 'panels', label: 'Panels', icon: 'panels' },
  { id: 'streams', label: 'Streams', icon: 'stream' },
];

// Tool views take over the whole main area with their own fixed layout (no dock panels). They sit
// below the primary views in the activity bar and hide the side bar while active.
export const TOOL_VIEWS: ViewDef[] = [
  { id: 'deck', label: 'Broadcast', icon: 'deck' },
  { id: 'studio', label: 'Studio', icon: 'studio' },
];

const TOOL_IDS = new Set<ViewId>(TOOL_VIEWS.map((v) => v.id));

// isToolView reports whether a view replaces the dock with a full-bleed tool surface.
export function isToolView(id: ViewId): boolean {
  return TOOL_IDS.has(id);
}

// The Panels catalog is the panel contribution registry's catalog-listed entries — one source of
// truth for both the sidebar list and how the dock renders each kind (panels/registry).
export { PANEL_CATALOG, type PanelContribution } from '../panels/registry';
