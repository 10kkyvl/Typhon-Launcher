<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import SourceBadge from '../../lib/components/SourceBadge.svelte';
  import type { GameArt } from '../../lib/services/metadata';
  import { gameContextMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { ambientHover } from '../../lib/utils/ambientHover';
  import { entryCover, type LibraryEntry } from '../../lib/library/entries';
  import { entryMeta, entrySubtitle } from './entryText';

  let {
    entries,
    art,
    switched = $bindable(false),
    element = $bindable(),
  }: {
    entries: readonly LibraryEntry[];
    art: Readonly<Record<string, GameArt>>;
    /** The view has just changed to this one: it rises in once. */
    switched?: boolean;
    element?: HTMLElement;
  } = $props();

  function endViewSwitch(event: AnimationEvent) {
    if (event.target === event.currentTarget) switched = false;
  }
</script>

<div class="list" class:switched onanimationend={endViewSwitch} bind:this={element}>
  {#each entries as entry (entry.id)}
    <button
      class="list-row"
      use:ambientHover={entry.id}
      onclick={() => navigate('game', { id: entry.id })}
      {@attach gameContextMenu(entry.id)}
    >
      <div class="list-thumb" aria-hidden="true">
        <Artwork src={entryCover(entry, art)} alt="" label={entry.title} radius="var(--radius-xs)" />
      </div>
      <span class="list-title">{entry.title}</span>
      {#if entry.platform}
        <ConsoleTag platform={entry.platform} />
      {:else if entry.source}
        <SourceBadge source={entry.source} />
      {/if}
      <span class="list-meta">{entrySubtitle(entry)}</span>
      <span class="list-meta right">{entry.kind === 'watched' ? '' : entryMeta(entry)}</span>
    </button>
  {/each}
</div>

<style>
  .list {
    display: flex;
    flex-direction: column;
  }

  .list.switched {
    animation: rise-in var(--dur-panel) var(--ease);
  }

  .list-row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 0.8rem 0.8rem;
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .list-row + .list-row {
    border-top: 1px solid var(--border);
  }

  .list-row:hover {
    background: var(--hover);
  }

  .list-thumb {
    width: 3.2rem;
    height: 4.2rem;
    flex-shrink: 0;
    border-radius: var(--radius-xs);
    overflow: hidden;
  }

  .list-title {
    flex: 1;
    min-width: 0;
    font-size: var(--font-md);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .list-meta {
    font-size: var(--font-sm);
    color: var(--text-3);
    white-space: nowrap;
  }

  .list-meta.right {
    min-width: 6rem;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
</style>
