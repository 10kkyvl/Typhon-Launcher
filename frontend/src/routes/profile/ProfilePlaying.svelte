<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import type { PlayingEntry } from '../../lib/services/profile';
  import { recentLabel } from '../../lib/profile/view';
  import { navigate } from '../../lib/stores/router';
  import HiddenBadge from './HiddenBadge.svelte';

  let { entries, hidden }: { entries: PlayingEntry[]; hidden: boolean } = $props();
</script>

{#if entries.length > 0}
  <section class="group">
    <div class="group-head">
      <h3>Сейчас играю</h3>
      {#if hidden}<HiddenBadge />{/if}
    </div>
    <ul class="rows">
      {#each entries as entry (entry.game.id)}
        <li>
          <button class="row" aria-label={entry.game.title} onclick={() => navigate('game', { id: entry.game.id })}>
            <span class="cover">
              <Artwork src={entry.game.cover} alt="" ratio="3 / 4" radius="var(--radius-sm)" />
            </span>
            <span class="title">{entry.game.title}</span>
            <span class="time">{recentLabel(entry.recentSeconds)}</span>
          </button>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .group {
    margin-bottom: var(--space-10);
  }

  .group-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }

  h3 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .rows {
    list-style: none;
    display: flex;
    flex-direction: column;
  }

  .rows li + li .row {
    border-top: 1px solid var(--border);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    width: 100%;
    padding: 0.8rem;
    background: none;
    border: 0;
    border-radius: var(--radius-md);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .cover {
    width: 3.6rem;
    flex-shrink: 0;
  }

  .title {
    flex: 1;
    min-width: 0;
    font-size: var(--font-md);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .time {
    font-size: var(--font-sm);
    color: var(--text-3);
    white-space: nowrap;
  }
</style>
