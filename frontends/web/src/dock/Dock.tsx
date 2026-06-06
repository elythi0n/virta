import { DockviewReact, type DockviewReadyEvent, type IDockviewPanelProps } from 'dockview';
import Panel from '../panels/Panel';
import Settings from '../panels/Settings';

// This module is the only place that imports dockview. The rest of the app receives the api
// through onReady and never depends on vendor shapes directly, so the engine stays swappable.

// Feed-like kinds share one renderer (the kind travels in params); Settings is its own panel.
const components = {
  panel: (props: IDockviewPanelProps<{ kind: string }>) => <Panel kind={props.params.kind} />,
  settings: () => <Settings />,
};

export default function Dock({ onReady }: { onReady: (event: DockviewReadyEvent) => void }) {
  // className selects our token-driven theme; the vendor theme is never imported.
  return <DockviewReact className="dockview-theme-virta" components={components} onReady={onReady} />;
}
