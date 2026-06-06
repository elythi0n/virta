import { useCallback, useEffect, useRef, useState } from 'react';
import type { DockviewApi, DockviewReadyEvent } from 'dockview';
import Dock from './dock/Dock';
import ActivityBar from './shell/ActivityBar';
import SideBar from './shell/SideBar';
import { TooltipProvider } from '@virta/ui-kit';
import type { ViewId } from './shell/views';
import { ThemeProvider } from './theme';

export default function App() {
  const [activeView, setActiveView] = useState<ViewId>('panels');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [theme, setTheme] = useState('graphite-dark');
  const apiRef = useRef<DockviewApi | null>(null);

  // Reflect the chosen theme onto the document so the token theme blocks switch.
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  const onReady = useCallback((event: DockviewReadyEvent) => {
    const api = event.api;
    apiRef.current = api;
    if (api.panels.length > 0) return; // idempotent under React StrictMode remounts

    const add = (id: string, kind: string, title: string, position?: Parameters<DockviewApi['addPanel']>[0]['position']) =>
      api.addPanel({ id, component: 'panel', params: { kind }, title, position });

    // Seed a representative workspace: a left group with the feed + mod queue as tabs, and a
    // right column splitting stats over the stream. Proves tabs, split, resize, drag rearrange.
    add('feed', 'feed', 'Unified feed');
    add('mods', 'mods', 'Mod queue', { referencePanel: 'feed', direction: 'within' });
    add('stats', 'stats', 'Stats', { referencePanel: 'feed', direction: 'right' });
    add('stream', 'stream', 'Stream', { referencePanel: 'stats', direction: 'below' });
  }, []);

  const selectView = useCallback(
    (view: ViewId) => {
      // Re-selecting the active view toggles the side bar, matching VS Code's activity bar.
      if (view === activeView) {
        setSidebarOpen((open) => !open);
      } else {
        setActiveView(view);
        setSidebarOpen(true);
      }
    },
    [activeView],
  );

  const openPanel = useCallback((kind: string, title: string) => {
    const api = apiRef.current;
    if (!api) return;
    const existing = api.getPanel(kind);
    if (existing) {
      existing.api.setActive(); // focus an already-open panel instead of duplicating it
      return;
    }
    api.addPanel({ id: kind, component: 'panel', params: { kind }, title });
  }, []);

  // Settings opens as its own dock panel (not a side-bar pane): it needs room for many
  // categories. Re-invoking focuses the existing panel.
  const openSettings = useCallback(() => {
    const api = apiRef.current;
    if (!api) return;
    const existing = api.getPanel('settings');
    if (existing) {
      existing.api.setActive();
      return;
    }
    api.addPanel({ id: 'settings', component: 'settings', title: 'Settings' });
  }, []);

  return (
    <ThemeProvider value={{ theme, setTheme }}>
      <TooltipProvider>
        <div className="shell">
          <ActivityBar activeView={activeView} sidebarOpen={sidebarOpen} onSelect={selectView} onOpenSettings={openSettings} />
          {sidebarOpen && <SideBar view={activeView} openPanel={openPanel} />}
          <div className="dock-host">
            <Dock onReady={onReady} />
          </div>
        </div>
      </TooltipProvider>
    </ThemeProvider>
  );
}
