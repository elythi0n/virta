<script lang="ts">
  import Icon from '../Icon.svelte';
  import { PRIMARY_VIEWS, FOOTER_VIEWS, type ViewId } from './views';

  let {
    activeView,
    sidebarOpen,
    onselect,
  }: { activeView: ViewId; sidebarOpen: boolean; onselect: (v: ViewId) => void } = $props();

  const isActive = (id: ViewId) => sidebarOpen && activeView === id;
</script>

<nav class="bar" aria-label="Primary">
  <div class="group">
    {#each PRIMARY_VIEWS as v (v.id)}
      <button
        class="item"
        class:active={isActive(v.id)}
        title={v.label}
        aria-label={v.label}
        aria-pressed={isActive(v.id)}
        onclick={() => onselect(v.id)}
      >
        <Icon name={v.icon} />
      </button>
    {/each}
  </div>
  <div class="group">
    {#each FOOTER_VIEWS as v (v.id)}
      <button
        class="item"
        class:active={isActive(v.id)}
        title={v.label}
        aria-label={v.label}
        aria-pressed={isActive(v.id)}
        onclick={() => onselect(v.id)}
      >
        <Icon name={v.icon} />
      </button>
    {/each}
  </div>
</nav>

<style>
  .bar {
    width: 48px;
    flex: none;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    background: var(--virta-bg-1);
    border-right: 1px solid var(--virta-line);
  }
  .group {
    display: flex;
    flex-direction: column;
  }
  .item {
    position: relative;
    height: 46px;
    display: grid;
    place-items: center;
    padding: 0;
    background: none;
    border: 0;
    color: var(--virta-text-2);
    cursor: pointer;
    transition: color var(--virta-motion-fast) ease;
  }
  .item:hover {
    color: var(--virta-text-1);
  }
  .item.active {
    color: var(--virta-text-0);
  }
  .item.active::before {
    content: '';
    position: absolute;
    left: 0;
    top: 9px;
    bottom: 9px;
    width: 2px;
    background: var(--virta-accent);
    border-radius: 0 2px 2px 0;
  }
  .item:focus-visible {
    outline: 2px solid var(--virta-accent);
    outline-offset: -3px;
    border-radius: var(--virta-radius-sm);
  }
</style>
