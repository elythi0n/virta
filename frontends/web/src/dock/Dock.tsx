import { DockviewReact, type DockviewDidDropEvent, type DockviewReadyEvent, type DockviewWillDropEvent, type IDockviewPanelProps } from 'dockview';
import Panel from '../panels/Panel';
import Settings from '../panels/Settings';
import HeaderActions from './HeaderActions';
import Tab from './Tab';

// This module is the only place that imports dockview. The rest of the app receives the api
// through onReady and never depends on vendor shapes directly, so the engine stays swappable.

const DRAG_KEY = 'virta/panel';

// Feed-like kinds share one renderer (the kind + optional channel set travel in params); Settings
// is its own panel.
const components = {
  panel: (props: IDockviewPanelProps<{ kind: string; channels?: string[] }>) => (
    <Panel kind={props.params.kind} channels={props.params.channels} panelId={props.api.id} />
  ),
  settings: () => <Settings />,
};

// onWillDrop fires before any drop (internal tab moves + external HTML5 drops).
// We only intercept it to set up external panel creation; internal drops pass through.
function handleWillDrop(e: DockviewWillDropEvent) {
  const native = e.nativeEvent;
  if (!(native instanceof DragEvent)) return;
  const raw = native.dataTransfer?.getData(DRAG_KEY);
  if (!raw) return;
  // Prevent dockview from treating this as an internal panel rearrangement.
  // The actual addPanel call happens in onDidDrop.
  native.stopPropagation();
}

function handleDidDrop(e: DockviewDidDropEvent) {
  const native = e.nativeEvent;
  if (!(native instanceof DragEvent)) return;
  const raw = native.dataTransfer?.getData(DRAG_KEY);
  if (!raw) return;
  let spec: { kind: string; channels?: string[]; title?: string };
  try { spec = JSON.parse(raw); } catch { return; }
  const id = spec.channels?.length ? `channel-${spec.channels[0]}-${Date.now()}` : `panel-${Date.now()}`;
  e.api.addPanel({
    id,
    component: 'panel',
    params: { kind: spec.kind, channels: spec.channels, title: spec.title },
    title: spec.title ?? spec.kind,
    position: e.group ? { referenceGroup: e.group.id, direction: 'within' } : undefined,
  });
}

export default function Dock({ onReady }: { onReady: (event: DockviewReadyEvent) => void }) {
  // className selects our token-driven theme; the vendor theme is never imported.
  // rightHeaderActionsComponent adds the per-group pop-out control. Floating groups stay enabled
  // (dockview default), so a tab can also be dragged out to float and dragged back to re-dock.
  return (
    <DockviewReact
      className="dockview-theme-virta"
      components={components}
      defaultTabComponent={Tab}
      rightHeaderActionsComponent={HeaderActions}
      onReady={onReady}
      onWillDrop={handleWillDrop}
      onDidDrop={handleDidDrop}
    />
  );
}
