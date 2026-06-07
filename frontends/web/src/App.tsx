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
import { OpenChannelProvider } from './openChannel';
import { OpenStreamProvider } from './openStream';
import { OpenUnifiedChatProvider } from './openUnifiedChat';
import { HostedAuthProvider } from './daemon/hostedAuth';
import { DensityProvider } from './density';
import { FeedDisplayProvider } from './feedDisplay';
import { A11yProvider } from './a11y';
import { ThemeProvider, DARK_THEME, LIGHT_THEME, type ThemeMode } from './theme';

const loadBool = (key: string) => {
  try {
    return localStorage.getItem(key) === '1';
  } catch {
    return false;
  }
};
const saveBool = (key: string, v: boolean) => {
  try {
    localStorage.setItem(key, v ? '1' : '0');
  } catch {
    // storage unavailable; the preference just won't persist
  }
};

const prefersDark = () => typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches;
function loadMode(): ThemeMode {
  try {
    const v = localStorage.getItem('virta.appearance');
    if (v === 'system' || v === 'light' || v === 'dark') return v;
  } catch {
    // ignore
  }
  return 'system';
}

export default function App() {
  const [activeView, setActiveView] = useState<ViewId>('panels');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [mode, setModeState] = useState<ThemeMode>(loadMode);
  const [systemDark, setSystemDark] = useState(prefersDark);
  // Resolve the appearance mode to a concrete token theme; system follows the OS preference.
  const theme = mode === 'dark' ? DARK_THEME : mode === 'light' ? LIGHT_THEME : systemDark ? DARK_THEME : LIGHT_THEME;
  const setMode = useCallback((m: ThemeMode) => {
    setModeState(m);
    try {
      localStorage.setItem('virta.appearance', m);
    } catch {
      // storage unavailable; appearance just won't persist across reloads
    }
  }, []);
  const [density, setDensity] = useState<Density>('cozy');
  const [showTimestamps, setShowTimestamps] = useState(true);
  const [showDeleted, setShowDeletedState] = useState(() => loadBool('virta.showDeleted'));
  const setShowDeleted = useCallback((v: boolean) => {
    setShowDeletedState(v);
    saveBool('virta.showDeleted', v);
  }, []);
  const [quickReplies, setQuickRepliesState] = useState<string[]>(() => {
    try {
      return JSON.parse(localStorage.getItem('virta.quickReplies') ?? '[]');
    } catch {
      return [];
    }
  });
  const setQuickReplies = useCallback((v: string[]) => {
    setQuickRepliesState(v);
    try {
      localStorage.setItem('virta.quickReplies', JSON.stringify(v));
    } catch {
      // storage unavailable; snippets just won't persist
    }
  }, []);
  const [mentionNames, setMentionNamesState] = useState<string[]>(() => {
    try {
      return JSON.parse(localStorage.getItem('virta.mentionNames') ?? '[]');
    } catch {
      return [];
    }
  });
  const setMentionNames = useCallback((names: string[]) => {
    setMentionNamesState(names);
    try {
      localStorage.setItem('virta.mentionNames', JSON.stringify(names));
    } catch {
      // storage unavailable; names just won't persist across reloads
    }
  }, []);
  const [autoCalmRate, setAutoCalmRateState] = useState<number>(() => {
    const n = Number(localStorage.getItem('virta.autoCalmRate'));
    return Number.isFinite(n) && n > 0 ? n : 0;
  });
  const setAutoCalmRate = useCallback((v: number) => {
    setAutoCalmRateState(v);
    try {
      localStorage.setItem('virta.autoCalmRate', String(v));
    } catch {
      // storage unavailable; the threshold just won't persist
    }
  }, []);
  const [reduceMotion, setReduceMotionState] = useState(() => loadBool('virta.reduceMotion'));
  const setReduceMotion = useCallback((v: boolean) => {
    setReduceMotionState(v);
    saveBool('virta.reduceMotion', v);
  }, []);
  const [dyslexicFont, setDyslexicFontState] = useState(() => loadBool('virta.dyslexicFont'));
  const setDyslexicFont = useCallback((v: boolean) => {
    setDyslexicFontState(v);
    saveBool('virta.dyslexicFont', v);
  }, []);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const [newFeedOpen, setNewFeedOpen] = useState(false);
  const apiRef = useRef<DockviewApi | null>(null);

  // Reflect the accessibility preferences as document-level attributes the global CSS keys off.
  useEffect(() => {
    document.documentElement.dataset.reduceMotion = reduceMotion ? '1' : '0';
  }, [reduceMotion]);
  useEffect(() => {
    if (dyslexicFont) document.documentElement.dataset.font = 'dyslexic';
    else delete document.documentElement.dataset.font;
  }, [dyslexicFont]);

  // Track the OS color-scheme so "system" mode flips live when the OS theme changes.
  useEffect(() => {
    if (typeof matchMedia !== 'function') return;
    const mq = matchMedia('(prefers-color-scheme: dark)');
    const onChange = (e: MediaQueryListEvent) => setSystemDark(e.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  // Reflect the resolved theme onto the document so the token theme blocks switch.
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
      add('feed', 'feed', 'Chat');
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

  // Open a unified feed for one streamer across all their platforms. Uses a stable ID derived from
  // the sorted channel keys so a second click focuses the existing pane instead of duplicating.
  const openUnifiedChat = useCallback((channelKeys: string[], label: string) => {
    const api = apiRef.current;
    if (!api || channelKeys.length === 0) return;
    const id = channelKeys.length === 1
      ? `channel-${channelKeys[0]}`
      : `unified-${channelKeys.slice().sort().join('+')}`;
    const existing = api.getPanel(id);
    if (existing) { existing.api.setActive(); return; }
    api.addPanel({ id, component: 'panel', params: { kind: 'feed', channels: channelKeys, title: label }, title: label });
  }, []);

  // Open a single-channel feed pane from the streams rail. The id is stable per channel, so a
  // second click focuses the existing pane instead of opening a duplicate.
  const openChannel = useCallback((channelKey: string, label: string) => {
    const api = apiRef.current;
    if (!api) return;
    const id = `channel-${channelKey}`;
    const existing = api.getPanel(id);
    if (existing) {
      existing.api.setActive();
      return;
    }
    api.addPanel({ id, component: 'panel', params: { kind: 'feed', channels: [channelKey], title: label }, title: label });
  }, []);

  // Open a channel's embedded player. Stable id per channel so it focuses rather than duplicates.
  const openStream = useCallback((channelKey: string, label: string) => {
    const api = apiRef.current;
    if (!api) return;
    const id = `watch-${channelKey}`;
    const existing = api.getPanel(id);
    if (existing) {
      existing.api.setActive();
      return;
    }
    api.addPanel({ id, component: 'panel', params: { kind: 'watch', channels: [channelKey], title: label }, title: label });
  }, []);

  // Existing scoped feeds (the "unified chats"), so the streams rail can merge a channel into one.
  const listFeeds = useCallback((): { id: string; title: string }[] => {
    const api = apiRef.current;
    if (!api) return [];
    return api.panels
      .filter((p) => {
        const params = p.params as { kind?: string; channels?: unknown } | undefined;
        return params?.kind === 'feed' && Array.isArray(params.channels);
      })
      .map((p) => ({ id: p.id, title: p.title ?? p.id }));
  }, []);

  // Merge a channel into an existing feed by adding it to that panel's channel set.
  const mergeChannelIntoFeed = useCallback((panelId: string, channelKey: string) => {
    const api = apiRef.current;
    if (!api) return;
    const panel = api.getPanel(panelId);
    if (!panel) return;
    const params = (panel.params ?? {}) as { kind?: string; channels?: string[]; title?: string };
    const channels = Array.isArray(params.channels) ? params.channels : [];
    if (!channels.includes(channelKey)) {
      panel.api.updateParameters({ ...params, channels: [...channels, channelKey] });
    }
    panel.api.setActive();
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
      { id: 'view-streams', title: 'Show Streams', group: 'View', perform: () => { setActiveView('streams'); setSidebarOpen(true); } },
      { id: 'toggle-sidebar', title: 'Toggle Side Bar', group: 'View', keywords: ['hide', 'show'], shortcut: 'mod+b', perform: () => setSidebarOpen((o) => !o) },
      { id: 'theme-system', title: 'Appearance: Follow system', group: 'Preferences', perform: () => setMode('system') },
      { id: 'theme-dark', title: 'Appearance: Dark', group: 'Preferences', perform: () => setMode('dark') },
      { id: 'theme-light', title: 'Appearance: Light', group: 'Preferences', perform: () => setMode('light') },
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
    <HostedAuthProvider>
    <ThemeProvider value={{ mode, setMode, theme }}>
      <A11yProvider value={{ reduceMotion, setReduceMotion, dyslexicFont, setDyslexicFont }}>
      <DensityProvider value={{ density, setDensity }}>
        <FeedDisplayProvider
          value={{ showTimestamps, setShowTimestamps, mentionNames, setMentionNames, showDeleted, setShowDeleted, quickReplies, setQuickReplies, autoCalmRate, setAutoCalmRate }}
        >
        <ActionsProvider value={actions}>
          <OpenChannelProvider value={openChannel}>
          <OpenStreamProvider value={openStream}>
          <OpenUnifiedChatProvider value={openUnifiedChat}>
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
            {sidebarOpen && (
              <SideBar
                view={activeView}
                openPanel={openPanel}
                openChannel={openChannel}
                openStream={openStream}
                listFeeds={listFeeds}
                mergeChannelIntoFeed={mergeChannelIntoFeed}
                onNewFeed={() => setNewFeedOpen(true)}
              />
            )}
            <div className="dock-host">
              <Dock onReady={onReady} />
            </div>
          </div>
        </div>
          <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} actions={actions} placeholder="Search commands…" />
          <ShortcutHelp open={helpOpen} onOpenChange={setHelpOpen} actions={actions} />
          <NewFeedDialog open={newFeedOpen} onClose={() => setNewFeedOpen(false)} onSubmit={openFeedSet} />
          </TooltipProvider>
          </OpenUnifiedChatProvider>
          </OpenStreamProvider>
          </OpenChannelProvider>
        </ActionsProvider>
        </FeedDisplayProvider>
      </DensityProvider>
      </A11yProvider>
    </ThemeProvider>
    </HostedAuthProvider>
  );
}
