<script lang="ts">
  import Icon from '../Icon.svelte';
  import { PANEL_CATALOG, SOURCES, type ViewId } from './views';

  let {
    view,
    theme,
    openPanel,
    setTheme,
  }: {
    view: ViewId;
    theme: string;
    openPanel: (kind: string, title: string) => void;
    setTheme: (t: string) => void;
  } = $props();

  const TITLES: Record<ViewId, string> = {
    panels: 'Panels',
    sources: 'Sources',
    settings: 'Settings',
  };

  const THEMES = [
    { id: 'graphite-dark', label: 'Graphite' },
    { id: 'light', label: 'Light' },
  ];
</script>

<aside class="side" aria-label={TITLES[view]}>
  <header class="head">{TITLES[view]}</header>
  <div class="body">
    {#if view === 'panels'}
      <ul class="rows">
        {#each PANEL_CATALOG as p (p.kind)}
          <li>
            <button class="row" onclick={() => openPanel(p.kind, p.title)}>
              <span class="glyph"><Icon name={p.icon} size={16} /></span>
              <span class="label">{p.title}</span>
            </button>
          </li>
        {/each}
      </ul>
    {:else if view === 'sources'}
      <ul class="rows">
        {#each SOURCES as s (s.id)}
          <li>
            <div class="row static">
              <span class="rail" style="background: {s.accent}"></span>
              <span class="label">{s.label}</span>
              <span class="state">Not connected</span>
            </div>
          </li>
        {/each}
      </ul>
      <p class="note">Connecting accounts wires to the daemon in a later step.</p>
    {:else if view === 'settings'}
      <div class="field">
        <span class="field-label">Appearance</span>
        <div class="segmented" role="group" aria-label="Theme">
          {#each THEMES as t (t.id)}
            <button
              class="seg"
              class:on={theme === t.id}
              aria-pressed={theme === t.id}
              onclick={() => setTheme(t.id)}>{t.label}</button
            >
          {/each}
        </div>
      </div>
    {/if}
  </div>
</aside>

<style>
  .side {
    width: 264px;
    flex: none;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--virta-bg-1);
    border-right: 1px solid var(--virta-line);
  }
  .head {
    flex: none;
    padding: var(--virta-space-12) var(--virta-space-16) var(--virta-space-8);
    color: var(--virta-text-2);
    font-size: var(--virta-type-meta-size);
    line-height: var(--virta-type-meta-line);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .body {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    padding: var(--virta-space-4) var(--virta-space-8) var(--virta-space-12);
  }
  .rows {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--virta-space-8);
    width: 100%;
    padding: var(--virta-space-8);
    border: 0;
    border-radius: var(--virta-radius-sm);
    background: none;
    color: var(--virta-text-1);
    font-family: var(--virta-font-ui);
    font-size: var(--virta-type-ui-size);
    line-height: var(--virta-type-ui-line);
    text-align: left;
    cursor: pointer;
    transition:
      background var(--virta-motion-fast) ease,
      color var(--virta-motion-fast) ease;
  }
  button.row:hover {
    background: var(--virta-bg-2);
    color: var(--virta-text-0);
  }
  button.row:focus-visible {
    outline: 2px solid var(--virta-accent);
    outline-offset: -2px;
  }
  .row.static {
    cursor: default;
  }
  .glyph {
    display: grid;
    place-items: center;
    color: var(--virta-text-2);
  }
  .label {
    flex: 1 1 auto;
    min-width: 0;
  }
  .rail {
    width: 3px;
    height: 16px;
    border-radius: 2px;
    flex: none;
  }
  .state {
    color: var(--virta-text-2);
    font-size: var(--virta-type-meta-size);
  }
  .note {
    margin: var(--virta-space-12) var(--virta-space-8) 0;
    color: var(--virta-text-2);
    font-size: var(--virta-type-meta-size);
    line-height: var(--virta-type-meta-line);
  }

  .field {
    padding: var(--virta-space-8);
  }
  .field-label {
    display: block;
    margin-bottom: var(--virta-space-8);
    color: var(--virta-text-2);
    font-size: var(--virta-type-meta-size);
  }
  .segmented {
    display: inline-flex;
    padding: 2px;
    gap: 2px;
    background: var(--virta-bg-0);
    border: 1px solid var(--virta-line);
    border-radius: var(--virta-radius-md);
  }
  .seg {
    padding: var(--virta-space-4) var(--virta-space-12);
    border: 0;
    border-radius: var(--virta-radius-sm);
    background: none;
    color: var(--virta-text-2);
    font-family: var(--virta-font-ui);
    font-size: var(--virta-type-ui-size);
    cursor: pointer;
    transition:
      background var(--virta-motion-fast) ease,
      color var(--virta-motion-fast) ease;
  }
  .seg:hover {
    color: var(--virta-text-1);
  }
  .seg.on {
    background: var(--virta-bg-2);
    color: var(--virta-text-0);
  }
  .seg:focus-visible {
    outline: 2px solid var(--virta-accent);
    outline-offset: 1px;
  }
</style>
