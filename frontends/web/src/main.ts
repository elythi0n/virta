import '../../ui-kit/tokens.css';
import 'dockview-core/dist/styles/dockview.css';
import './app.css';
import './dock-theme.css';

import { mount, unmount } from 'svelte';
import { createDock } from './dock/dock';
import Panel from './panels/Panel.svelte';

const target = document.getElementById('app');
if (!target) throw new Error('missing #app mount target');

const dock = createDock(target, {
  render(host, kind) {
    const cmp = mount(Panel, { target: host, props: { kind } });
    return () => {
      void unmount(cmp);
    };
  },
});

// Seed a representative workspace: a left group with the feed + mod queue as tabs, and a right
// column splitting stats over the stream. Proves tabs, split, resize, and drag rearrange.
dock.add({ id: 'feed', kind: 'feed', title: 'Unified feed' });
dock.add({ id: 'mods', kind: 'mods', title: 'Mod queue', position: { referencePanelId: 'feed', direction: 'within' } });
dock.add({ id: 'stats', kind: 'stats', title: 'Stats', position: { referencePanelId: 'feed', direction: 'right' } });
dock.add({ id: 'stream', kind: 'stream', title: 'Stream', position: { referencePanelId: 'stats', direction: 'below' } });
