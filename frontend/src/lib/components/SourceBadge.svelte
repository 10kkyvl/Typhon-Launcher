<script lang="ts">
  import { HardDrive } from '@lucide/svelte';
  import { sourceLabel, sourceTitle, type GameSource } from '../library/source';

  let { source }: { source: GameSource } = $props();
</script>

<span class="badge" data-source={source} title={sourceTitle(source)}>
  {#if source === 'disk'}
    <HardDrive size="1.2rem" strokeWidth={2} />
  {/if}
  {sourceLabel(source)}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    max-width: 100%;
    padding: 0.25rem 0.55rem;
    border-radius: var(--radius-sm);
    background: rgba(5, 8, 12, 0.72);
    color: #fff;
    font-size: 1.1rem;
    line-height: 1.2;
    white-space: nowrap;
  }

  .badge[data-source='typhon'] {
    background: var(--accent);
    color: var(--accent-on, #fff);
  }

  .badge[data-source='steam'] {
    background: #1b2838;
    color: #66c0f4;
  }

  .badge[data-source='epic'] {
    background: #2a2a2a;
  }

  .badge[data-source='gog'] {
    background: #6b2c91;
  }
</style>
