import { useMemo, useState, useSyncExternalStore } from 'react';
import type { IDockviewHeaderActionsProps } from 'dockview';
import { Popover, Tooltip } from '@virta/ui-kit';
import Icon from '../Icon';
import { PANEL_CATALOG } from '../shell/views';
import {
  PANEL_GROUPS,
  panelCatalogVersion,
  subscribePanelCatalog,
  type PanelContribution,
  type PanelGroup,
} from '../panels/registry';
import { useTheme } from '../theme';
import { useIsDesktop } from '../shell/useIsDesktop';
import styles from './HeaderActions.module.css';

const uid = () => `panel-${crypto.randomUUID?.() ?? Math.random().toString(36).slice(2)}`;

// Right-side header controls for a group: a "+" to add a panel as a tab in this pane, and (while
// the group is docked in the grid) a pop-out into its own window.
export default function HeaderActions(props: IDockviewHeaderActionsProps) {
  const { theme } = useTheme();
  // Re-render the "+" catalog when plugin panels register at runtime.
  useSyncExternalStore(subscribePanelCatalog, panelCatalogVersion, panelCatalogVersion);
  const [addOpen, setAddOpen] = useState(false);
  const isDesktop = useIsDesktop();
  const inGrid = !props.location || props.location.type === 'grid';

  const addPanel = (kind: string, title: string) => {
    setAddOpen(false);
    props.containerApi.addPanel({
      id: uid(),
      component: 'panel',
      params: { kind },
      title,
      position: { referenceGroup: props.group, direction: 'within' },
    });
  };

  // Group the catalog so the popover renders section headers (Chat / Stream / Broadcast / Tools)
  // instead of one long flat list. Anything without a group falls into "Plugins" at the bottom,
  // which is where third-party contributions end up by default.
  const sections = useMemo(() => {
    const by: Record<PanelGroup, PanelContribution[]> = {
      Chat: [], Stream: [], Broadcast: [], Tools: [], Plugins: [],
    };
    for (const p of PANEL_CATALOG) {
      const g = (p.group ?? 'Plugins') as PanelGroup;
      (by[g] ?? by.Plugins).push(p);
    }
    return PANEL_GROUPS.filter((g) => by[g].length > 0).map((g) => ({ group: g, items: by[g] }));
  }, []);

  const popOut = () => {
    void props.containerApi.addPopoutGroup(props.group, {
      onDidOpen: ({ window }) => {
        window.document.documentElement.dataset.theme = theme;
      },
    });
  };

  return (
    <div className={styles.actions}>
      <Popover
        open={addOpen}
        onOpenChange={setAddOpen}
        align="end"
        trigger={
          <button className={styles.action} aria-label="Add a tab to this pane">
            <Icon name="plus" size={15} />
          </button>
        }
      >
        <div className={styles.menu} role="menu" aria-label="Add a tab">
          {sections.map((s, idx) => (
            <div key={s.group} className={styles.section}>
              {idx > 0 && <div className={styles.separator} role="presentation" />}
              <div className={styles.sectionLabel} role="presentation">{s.group}</div>
              {s.items.map((p) => (
                <button
                  key={p.kind}
                  type="button"
                  className={styles.menuItem}
                  onClick={() => addPanel(p.kind, p.title)}
                >
                  <span className={styles.menuGlyph}>
                    <Icon name={p.icon} size={15} />
                  </span>
                  {p.title}
                </button>
              ))}
            </div>
          ))}
        </div>
      </Popover>

      {inGrid && !isDesktop && (
        <Tooltip content="Pop out to window" side="bottom">
          <button className={styles.action} aria-label="Pop out to window" onClick={popOut}>
            <Icon name="popout" size={15} />
          </button>
        </Tooltip>
      )}
    </div>
  );
}
