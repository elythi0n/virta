<script lang="ts">
  let { kind }: { kind: string } = $props();

  type Row = { id: number; ts: string; plat: 'twitch' | 'kick'; author: string; body: string };

  // A long synthetic feed to eyeball scroll smoothness and verify the body keeps rendering
  // through a drag/resize. Real virtualization and live data arrive with feed-core (M4.2).
  const SAMPLE = [
    'gg that was clean',
    'no shot he hit that',
    'first time catching the stream live',
    'what headset is that',
    'chat is going feral lol',
    'W player',
    'the pacing on this run is unreal',
  ];
  const rows: Row[] = Array.from({ length: 600 }, (_, i) => ({
    id: i,
    ts: `${String(12 + Math.floor(i / 60)).padStart(2, '0')}:${String(i % 60).padStart(2, '0')}`,
    plat: i % 3 === 0 ? 'kick' : 'twitch',
    author: `viewer_${(i * 7) % 240}`,
    body: SAMPLE[i % SAMPLE.length],
  }));
</script>

{#if kind === 'feed'}
  <div class="feed" role="log" aria-label="Unified feed">
    {#each rows as row (row.id)}
      <div class="row">
        <span class="ts">{row.ts}</span>
        <span class="dot" class:kick={row.plat === 'kick'}></span>
        <span class="author">{row.author}</span>
        <span class="body">{row.body}</span>
      </div>
    {/each}
  </div>
{:else}
  <div class="placeholder">
    <span class="label">{kind}</span>
    <span class="hint">panel content lands in a later milestone</span>
  </div>
{/if}

<style>
  .feed {
    height: 100%;
    overflow-y: auto;
    padding: var(--virta-space-4) 0;
    font-family: var(--virta-font-ui);
  }
  .row {
    display: grid;
    grid-template-columns: auto auto auto 1fr;
    align-items: baseline;
    gap: var(--virta-space-8);
    padding: var(--virta-space-2) var(--virta-space-12);
    font-size: var(--virta-type-chat-compact-size);
    line-height: var(--virta-type-chat-compact-line);
  }
  .row:hover {
    background: var(--virta-bg-1);
  }
  .ts {
    color: var(--virta-text-2);
    font-family: var(--virta-font-mono);
    font-size: var(--virta-type-meta-size);
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--virta-plat-twitch);
    transform: translateY(-1px);
  }
  .dot.kick {
    background: var(--virta-plat-kick);
  }
  .author {
    color: var(--virta-text-1);
    font-weight: var(--virta-type-chat-compact-weight);
  }
  .body {
    color: var(--virta-text-0);
  }

  .placeholder {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--virta-space-4);
    height: 100%;
    font-family: var(--virta-font-ui);
  }
  .label {
    color: var(--virta-text-0);
    font-size: var(--virta-type-title-size);
    font-weight: var(--virta-type-title-weight);
    text-transform: capitalize;
  }
  .hint {
    color: var(--virta-text-2);
    font-size: var(--virta-type-meta-size);
  }
</style>
