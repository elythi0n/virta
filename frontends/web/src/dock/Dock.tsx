import { DockviewReact, type DockviewReadyEvent, type IDockviewPanelProps } from 'dockview';
import Panel from '../panels/Panel';
import Settings from '../panels/Settings';
import HeaderActions from './HeaderActions';

// This module is the only place that imports dockview. The rest of the app receives the api
// through onReady and never depends on vendor shapes directly, so the engine stays swappable.

// Feed-like kinds share one renderer (the kind + optional channel set travel in params); Settings
// is its own panel.
const components = {
  panel: (props: IDockviewPanelProps<{ kind: string; channels?: string[] }>) => (
    <Panel kind={props.params.kind} channels={props.params.channels} />
  ),
  settings: () => <Settings />,
};

export default function Dock({ onReady }: { onReady: (event: DockviewReadyEvent) => void }) {
  // className selects our token-driven theme; the vendor theme is never imported.
  // rightHeaderActionsComponent adds the per-group pop-out control. Floating groups stay enabled
  // (dockview default), so a tab can also be dragged out to float and dragged back to re-dock.
  return (
    <DockviewReact
      className="dockview-theme-virta"
      components={components}
      rightHeaderActionsComponent={HeaderActions}
      onReady={onReady}
    />
  );
}
