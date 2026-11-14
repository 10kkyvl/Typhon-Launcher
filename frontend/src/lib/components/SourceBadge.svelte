<script lang="ts">
  import { HardDrive } from '@lucide/svelte';
  import Tooltip from './Tooltip.svelte';
  import { sourceLabel, sourceTitle, type GameSource } from '../library/source';

  let { source }: { source: GameSource } = $props();
</script>

<!-- The short label is shown; the full sentence is what a screen reader reads, and the hint on hover
     and keyboard focus. The hint repeats what is read, so it is not a description too. -->
<Tooltip text={sourceTitle(source)} describe={false}>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (A stop in the Tab order so the hint reaches keyboard users; pressing it does nothing.) -->
  <span class="badge" data-source={source} tabindex="0">
    {#if source === 'disk'}
      <HardDrive size="1.2rem" strokeWidth={2} aria-hidden="true" />
    {/if}
    <span aria-hidden="true">{sourceLabel(source)}</span>
    <span class="sr-only">{sourceTitle(source)}</span>
  </span>
</Tooltip>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    max-width: 100%;
    padding: 0.25rem 0.55rem;
    border-radius: var(--radius-sm);
    background: var(--source-badge-fill);
    color: var(--source-badge-text);
    font-size: var(--font-xs);
    line-height: 1.2;
    white-space: nowrap;
  }

  .badge[data-source='typhon'] {
    background: var(--accent);
    color: var(--accent-on, var(--source-badge-text));
  }

  .badge[data-source='steam'] {
    background: var(--source-steam-fill);
    color: var(--source-steam-text);
  }

  .badge[data-source='epic'] {
    background: var(--source-epic-fill);
  }

  .badge[data-source='gog'] {
    background: var(--source-gog-fill);
  }
</style>
