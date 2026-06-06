import { createDockview, type DockviewApi, type IContentRenderer } from 'dockview-core';

// This module is the only place that imports dockview-core. Everything else talks to the Dock
// interface, so the mechanics provider stays swappable and the app never depends on vendor shapes.

export type DockDirection = 'left' | 'right' | 'above' | 'below' | 'within';

export interface DockPanel {
  id: string;
  /** Selects which content renderer builds the body. */
  kind: string;
  /** Tab label. */
  title: string;
  /** Where to seed the panel relative to an existing one; omitted = first/root group. */
  position?: { referencePanelId: string; direction: DockDirection };
}

export interface DockOptions {
  /** Mount the panel body into `host`; return a disposer to tear it down on close. */
  render: (host: HTMLElement, kind: string) => (() => void) | void;
}

export interface Dock {
  add(panel: DockPanel): void;
  /** Escape hatch to the underlying engine for spike-only experiments. */
  api: DockviewApi;
  dispose(): void;
}

export function createDock(target: HTMLElement, opts: DockOptions): Dock {
  const api = createDockview(target, {
    className: 'dockview-theme-virta',
    createComponent: (options): IContentRenderer => {
      const element = document.createElement('div');
      element.className = 'dock-panel';
      let dispose: (() => void) | void;
      return {
        element,
        init() {
          dispose = opts.render(element, options.name);
        },
        dispose() {
          dispose?.();
        },
      };
    },
  });

  return {
    api,
    add(panel) {
      api.addPanel({
        id: panel.id,
        component: panel.kind,
        title: panel.title,
        position: panel.position
          ? { referencePanel: panel.position.referencePanelId, direction: panel.position.direction }
          : undefined,
      });
    },
    dispose() {
      api.dispose();
    },
  };
}
