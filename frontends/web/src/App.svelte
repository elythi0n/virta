<script lang="ts">
  import { mount, unmount } from 'svelte';
  import { createDock, type Dock } from './dock/dock';
  import Panel from './panels/Panel.svelte';
  import ActivityBar from './shell/ActivityBar.svelte';
  import SideBar from './shell/SideBar.svelte';
  import type { ViewId } from './shell/views';

  let activeView = $state<ViewId>('panels');
  let sidebarOpen = $state(true);
  let theme = $state('graphite-dark');
  let dock: Dock | undefined;

  // Reflect the chosen theme onto the document so the token theme blocks switch.
  $effect(() => {
    document.documentElement.dataset.theme = theme;
  });

  // Svelte action: own the dock host element's lifecycle, seeding the initial layout once.
  function dockHost(node: HTMLElement) {
    const d = createDock(node, {
      render(el, kind) {
        const cmp = mount(Panel, { target: el, props: { kind } });
        return () => void unmount(cmp);
      },
    });
    d.add({ id: 'feed', kind: 'feed', title: 'Unified feed' });
    d.add({ id: 'mods', kind: 'mods', title: 'Mod queue', position: { referencePanelId: 'feed', direction: 'within' } });
    d.add({ id: 'stats', kind: 'stats', title: 'Stats', position: { referencePanelId: 'feed', direction: 'right' } });
    d.add({ id: 'stream', kind: 'stream', title: 'Stream', position: { referencePanelId: 'stats', direction: 'below' } });
    dock = d;
    return {
      destroy() {
        d.dispose();
        dock = undefined;
      },
    };
  }

  function selectView(view: ViewId) {
    // Re-selecting the active view toggles the side bar, matching VS Code's activity bar.
    if (view === activeView) {
      sidebarOpen = !sidebarOpen;
    } else {
      activeView = view;
      sidebarOpen = true;
    }
  }

  function openPanel(kind: string, title: string) {
    dock?.open({ id: kind, kind, title });
  }
</script>

<div class="shell">
  <ActivityBar {activeView} {sidebarOpen} onselect={selectView} />
  {#if sidebarOpen}
    <SideBar view={activeView} {theme} {openPanel} setTheme={(t) => (theme = t)} />
  {/if}
  <div class="dock-host" use:dockHost></div>
</div>

<style>
  .shell {
    display: flex;
    height: 100%;
    min-height: 0;
  }
  .dock-host {
    flex: 1 1 auto;
    min-width: 0;
    height: 100%;
  }
</style>
