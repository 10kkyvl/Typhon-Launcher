<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import Card from '../../lib/components/Card.svelte';
  import type { PlayingEntry } from '../../lib/services/profile';
  import { recentLabel } from '../../lib/profile/view';
  import { navigate } from '../../lib/stores/router';
  import HiddenBadge from './HiddenBadge.svelte';

  let { entries, hidden }: { entries: PlayingEntry[]; hidden: boolean } = $props();
</script>

{#if entries.length > 0}
  <Card>
    <div class="block-head">
      <h3>Сейчас играю</h3>
      {#if hidden}<HiddenBadge />{/if}
    </div>
    <ul class="rows">
      {#each entries as entry (entry.game.id)}
        <li>
          <button class="row" onclick={() => navigate('game', { id: entry.game.id })}>
            <span class="cover">
              <Artwork src={entry.game.cover} alt={entry.game.title} ratio="3 / 4" radius="var(--radius-sm)" />
            </span>
            <span class="title">{entry.game.title}</span>
            <span class="time">{recentLabel(entry.recentSeconds)}</span>
          </button>
        </li>
      {/each}
    </ul>
  </Card>
{/if}

<style>
  .block-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
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
    gap: var(--space-3);
    width: 100%;
    padding: 1rem 0;
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    border-radius: var(--radius-sm);
  }

  .row:hover .title {
    color: var(--accent-text);
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
    transition: color var(--dur) var(--ease);
  }

  .time {
    font-size: var(--font-sm);
    color: var(--text-2);
    white-space: nowrap;
  }
</style>
