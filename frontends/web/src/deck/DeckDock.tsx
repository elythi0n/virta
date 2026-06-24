import { useCallback } from 'react';
import {
  DockviewReact,
  type DockviewApi,
  type DockviewReadyEvent,
  type IDockviewPanelProps,
} from 'dockview';
import HeaderActions from '../dock/HeaderActions';
import Panel from '../panels/Panel';
import DeckTab from './DeckTab';
import StreamInfoPane from './panes/StreamInfoPane';
import { useDeck } from './DeckContext';
import { loadDeckLayout, saveDeckLayoutDebounced } from './deckLayout';
import styles from './DeckDock.module.css';

// The Broadcast workspace is its own dockview tree, separate from the main Panels dock. It uses
// the same panel components and registry, so any kind (chat, mods, mentions, stats, gifts, OBS
// controls, encoder health…) works inside it. Seeded panes carry `params.locked = true`, which
// DeckTab reads to hide the close affordance — additions made by the user via the group "+"
// don't carry that flag and stay closeable, so duplicates can be removed.

const components = {
  // Stream info depends on the deck-form context which only exists inside the Broadcast
  // workspace, so it isn't registered in the global panel catalog.
  'stream-info': () => <StreamInfoPane />,
  // Reuse the main panel router for everything else — chat, stats, mods, gifts, mentions, OBS
  // controls, encoder health — so the Broadcast workspace shares its content with the main app
  // and stays consistent when new panel kinds (or plugin panels) register.
  panel: (props: IDockviewPanelProps<{ kind: string; channels?: string[] }>) => (
    <Panel kind={props.params.kind} channels={props.params.channels} panelId={props.api.id} />
  ),
};

// Seed the workspace minimally — Stream Info on the left, Chat on the right. Both are locked
// (no close button); everything else (Stats, Gifts, Mod queue, Mentions, OBS controls, Encoder
// health) is one "+" click away on a group header. Starting empty keeps the workspace calm and
// lets the user opt into the panes they want instead of inheriting a busy default.
function seedLayout(api: DockviewApi, chatChannels: string[]) {
  api.addPanel({
    id: 'deck-stream-info',
    component: 'stream-info',
    // `kind` is metadata for DeckTab's icon lookup — it isn't routed by the registry because
    // stream-info is its own dockview component (not handled by the panel router).
    params: { kind: 'stream-info', locked: true },
    title: 'Stream info',
  });
  api.addPanel({
    id: 'deck-chat',
    component: 'panel',
    params: {
      kind: 'feed',
      channels: chatChannels.length ? chatChannels : undefined,
      locked: true,
    },
    title: 'Chat',
    position: { referencePanel: 'deck-stream-info', direction: 'right' },
  });
}

export default function DeckDock() {
  const { chatChannels } = useDeck();

  const onReady = useCallback(
    (event: DockviewReadyEvent) => {
      const api = event.api;
      const saved = loadDeckLayout();
      if (saved) {
        try {
          api.fromJSON(saved);
        } catch {
          api.clear();
        }
      }
      if (api.panels.length === 0) {
        seedLayout(api, chatChannels);
      }
      api.onDidLayoutChange(() => saveDeckLayoutDebounced(api));
    },
    [chatChannels],
  );

  return (
    <div className={styles.dockHost}>
      <DockviewReact
        className="dockview-theme-virta"
        components={components}
        defaultTabComponent={DeckTab}
        rightHeaderActionsComponent={HeaderActions}
        disableFloatingGroups
        onReady={onReady}
      />
    </div>
  );
}
