<script lang="ts">
  import { CheckCircle2, Heart, Play } from '@lucide/svelte';
  import Card from '../../lib/components/Card.svelte';
  import type { ActivityView } from '../../lib/services/social';
  import { eventLine } from '../../lib/social/feed';
  import { openGameByIGDB } from '../../lib/social/openGame';
  import { relativeDate } from '../../lib/utils/format';

  let { items }: { items: ActivityView[] } = $props();
</script>

<Card title="Недавняя активность">
  <ul class="list">
    {#each items as item (item.id)}
      <li>
        <button class="row" type="button" onclick={() => openGameByIGDB(item.game.igdbId, item.game.title)}>
          <span class="icon">
            {#if item.kind === 'completed'}
              <CheckCircle2 size="1.7rem" strokeWidth={1.8} />
            {:else if item.kind === 'favorited'}
              <Heart size="1.7rem" strokeWidth={1.8} />
            {:else}
              <Play size="1.7rem" strokeWidth={1.8} />
            {/if}
          </span>
          <span class="text">{eventLine(item)}</span>
          <span class="when">{relativeDate(item.createdAt)}</span>
        </button>
      </li>
    {/each}
  </ul>
</Card>

<style>
  .list {
    list-style: none;
    display: flex;
    flex-direction: column;
  }

  .list li + li .row {
    border-top: 1px solid var(--border);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.8rem;
    margin: 0 calc(var(--space-3) * -1);
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

  .icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.6rem;
    height: 3.6rem;
    flex-shrink: 0;
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .text {
    flex: 1;
    min-width: 0;
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .when {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }
</style>
