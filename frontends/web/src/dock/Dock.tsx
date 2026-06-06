import { DockviewReact, type DockviewReadyEvent, type IDockviewPanelProps } from 'dockview';
import Panel from '../panels/Panel';

// This module is the only place that imports dockview. The rest of the app receives the api
// through onReady and never depends on vendor shapes directly, so the engine stays swappable.

// Every panel kind is rendered by the same component; the kind travels in the panel params.
const components = {
  panel: (props: IDockviewPanelProps<{ kind: string }>) => <Panel kind={props.params.kind} />,
};

export default function Dock({ onReady }: { onReady: (event: DockviewReadyEvent) => void }) {
  // className selects our token-driven theme; the vendor theme is never imported.
  return <DockviewReact className="dockview-theme-virta" components={components} onReady={onReady} />;
}
