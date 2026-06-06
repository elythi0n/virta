import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { DockviewApi, DockviewReadyEvent } from 'dockview';
import Dock from './dock/Dock';
import ActivityBar from './shell/ActivityBar';
import SideBar from './shell/SideBar';
import Titlebar from './shell/Titlebar';
import ConnectionAlert from './shell/ConnectionAlert';
import ShortcutHelp from './shell/ShortcutHelp';
import NewFeedDialog from './shell/NewFeedDialog';
import { CommandPalette, TooltipProvider, matchesShortcut, type CommandAction } from '@virta/ui-kit';
import type { Density } from '@virta/feed-core';
import { PANEL_CATALOG, type ViewId } from './shell/views';
import { loadLayout, saveLayoutDebounced } from './shell/layout';
import { ActionsProvider } from './actions';
import { DensityProvider } from './density';
import { FeedDisplayProvider } from './feedDisplay';
import { ThemeProvider } from './theme';

export default function App() {
  const [activeView, setActiveView] = useState<ViewId>('panels');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [theme, setTheme] = useState('graphite-dark');
  const [density, setDensity] = useState<Density>('cozy');
  const [showTimestamps, setShowTimestamps] = useState(true);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const [newFeedOpen, setNewFeedOpen] = useState(false);
  const apiRef = useRef<DockviewApi | null>(null);

  // Reflect the chosen theme onto the document so the token theme blocks switch.
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  const onReady = useCallback((event: DockviewReadyEvent) => {
    const api = event.api;
    apiRef.current = api;

    // Restore the saved workspace; fall back to a seeded default if there's none (or it's stale).
    const saved = loadLayout();
    let restored = false;
    if (saved) {
      try {
        api.fromJSON(saved);
        restored = true;
      } catch {
        restored = false;
      }
    }
    if (!restored && api.panels.length === 0) {
      const add = (id: string, kind: string, title: string, position?: Parameters<DockviewApi['addPanel']>[0]['position']) =>
        api.addPanel({ id, component: 'panel', params: { kind }, title, position });
      // A representative default workspace: feed + mod queue as tabs, stats over stream beside it.
      add('feed', 'feed', 'Unified feed');
      add('mods', 'mods', 'Mod queue', { referencePanel: 'feed', direction: 'within' });
      add('stats', 'stats', 'Stats', { referencePanel: 'feed', direction: 'right' });
      add('stream', 'stream', 'Stream', { referencePanel: 'stats', direction: 'below' });
    }

    api.onDidLayoutChange(() => saveLayoutDebounced(api));
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

  // Open a feed scoped to a channel set as a new panel. The id is stable and unique (not derived
  // from the set) so the feed stays the same panel when its channels are later edited from the tab.
  const openFeedSet = useCallback((title: string, channels: string[]) => {
    const api = apiRef.current;
    if (!api || channels.length === 0) return;
    const id = `feedset-${crypto.randomUUID?.() ?? Math.random().toString(36).slice(2)}`;
    api.addPanel({ id, component: 'panel', params: { kind: 'feed', channels, title }, title });
  }, []);

  // Every shell action is registered here so the palette (and later, the keymap and menus)
  // dispatch through one list rather than each surface wiring its own handler.
  const actions = useMemo<CommandAction[]>(() => {
    const openPanels = PANEL_CATALOG.map((p) => ({
      id: `open-${p.kind}`,
      title: `Open ${p.title}`,
      group: 'Open',
      keywords: [p.kind],
      perform: () => openPanel(p.kind, p.title),
    }));
    return [
      { id: 'command-palette', title: 'Command Palette', group: 'Go', shortcut: 'mod+shift+p', perform: () => setPaletteOpen(true) },
      { id: 'shortcut-help', title: 'Keyboard Shortcuts', group: 'Help', shortcut: 'mod+/', perform: () => setHelpOpen(true) },
      ...openPanels,
      { id: 'new-feed', title: 'New Feed…', group: 'Open', keywords: ['set', 'channels', 'unified'], perform: () => setNewFeedOpen(true) },
      { id: 'open-settings', title: 'Open Settings', group: 'Open', shortcut: 'mod+,', perform: openSettings },
      { id: 'view-panels', title: 'Show Panels', group: 'View', perform: () => { setActiveView('panels'); setSidebarOpen(true); } },
      { id: 'view-sources', title: 'Show Sources', group: 'View', perform: () => { setActiveView('sources'); setSidebarOpen(true); } },
      { id: 'toggle-sidebar', title: 'Toggle Side Bar', group: 'View', keywords: ['hide', 'show'], shortcut: 'mod+b', perform: () => setSidebarOpen((o) => !o) },
      { id: 'theme-graphite', title: 'Theme: Graphite (Dark)', group: 'Preferences', perform: () => setTheme('graphite-dark') },
      { id: 'theme-light', title: 'Theme: Light', group: 'Preferences', perform: () => setTheme('light') },
    ];
  }, [openPanel, openSettings]);

  // One keymap: dispatch the first action whose shortcut matches. Skip when typing in a field.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName))) return;
      for (const a of actions) {
        if (a.shortcut && matchesShortcut(e, a.shortcut)) {
          e.preventDefault();
          a.perform();
          return;
        }
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [actions]);

  return (
    <ThemeProvider value={{ theme, setTheme }}>
      <DensityProvider value={{ density, setDensity }}>
        <FeedDisplayProvider value={{ showTimestamps, setShowTimestamps }}>
        <ActionsProvider value={actions}>
          <TooltipProvider>
          <div className="app">
          <Titlebar onOpenPalette={() => setPaletteOpen(true)} />
          <ConnectionAlert />
          <div className="shell">
            <ActivityBar
              activeView={activeView}
              sidebarOpen={sidebarOpen}
              onSelect={selectView}
              onOpenSettings={openSettings}
              onOpenPlugins={() => openPanel('plugins', 'Plugins')}
            />
            {sidebarOpen && <SideBar view={activeView} openPanel={openPanel} onNewFeed={() => setNewFeedOpen(true)} />}
            <div className="dock-host">
              <Dock onReady={onReady} />
            </div>
          </div>
        </div>
          <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} actions={actions} placeholder="Search commands…" />
          <ShortcutHelp open={helpOpen} onOpenChange={setHelpOpen} actions={actions} />
          <NewFeedDialog open={newFeedOpen} onClose={() => setNewFeedOpen(false)} onSubmit={openFeedSet} />
          </TooltipProvider>
        </ActionsProvider>
        </FeedDisplayProvider>
      </DensityProvider>
    </ThemeProvider>
  );
}
