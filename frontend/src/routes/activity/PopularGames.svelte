<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import Card from '../../lib/components/Card.svelte';
  import { openGameByIGDB } from '../../lib/social/openGame';
  import type { PopularGame } from '../../lib/social/popular';
  import { msg } from '../../lib/i18n';

  let { games }: { games: PopularGame[] } = $props();
</script>

<Card title={msg('transfers.activityPopularTitle')}>
  {#if games.length === 0}
    <p class="muted">{msg('transfers.activityPopularEmpty')}</p>
  {:else}
    <div class="list">
      {#each games as item (item.game.igdbId)}
        <button
          class="row"
          type="button"
          title={item.names.join(', ')}
          onclick={() => openGameByIGDB(item.game.igdbId, item.game.title)}
        >
          <span class="cover">
            <Artwork src={item.game.coverUrl} alt={item.game.title} ratio="3 / 4" radius="var(--radius-sm)" />
          </span>
          <span class="text">
            <span class="title">{item.game.title}</span>
            <span class="sub">
              {msg('transfers.activityPopularFriends', { count: item.count })}
              {#if item.playing > 0}
                <span class="now">· {msg('transfers.activityPopularPlayingNow', { count: item.playing })}</span>
              {/if}
            </span>
          </span>
        </button>
      {/each}
    </div>
  {/if}
</Card>

<style>
  .list {
    display: flex;
    flex-direction: column;
    margin: 0 calc(var(--space-4) * -1);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    width: 100%;
    padding: 0.8rem var(--space-4);
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .cover {
    display: block;
    flex-shrink: 0;
    width: 3.6rem;
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .title {
    font-size: var(--font-sm);
    font-weight: 600;
    line-height: 1.3;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub {
    font-size: 1.2rem;
    color: var(--text-3);
  }

  .now {
    color: var(--accent-text);
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
