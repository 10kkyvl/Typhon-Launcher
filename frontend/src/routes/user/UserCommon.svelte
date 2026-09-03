<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import type { CommonGames } from '../../lib/services/social';
  import { commonGameLabel, commonGamesTitle } from '../../lib/social/profileView';

  let { common, name }: { common: CommonGames; name: string } = $props();

  const rows = $derived(common.games.slice(0, 6));
</script>

<section class="group">
  <h3>Играете оба</h3>
  <p class="count">{commonGamesTitle(common.count)}</p>
  <div class="list">
    {#each rows as game (game.igdbId)}
      <div class="row">
        <div class="thumb">
          <Artwork src={game.coverUrl} alt={game.title} ratio="3 / 4" />
        </div>
        <span class="title">{game.title}</span>
        <span class="label">{commonGameLabel(game.viewerOwned, game.targetOwned, name)}</span>
      </div>
    {/each}
  </div>
</section>

<style>
  .group {
    margin-bottom: var(--space-10);
  }

  h3 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-2);
  }

  .count {
    font-size: var(--font-sm);
    color: var(--text-2);
    margin-bottom: var(--space-3);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 0.8rem;
    border-radius: var(--radius-md);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .thumb {
    width: 3.2rem;
    height: 4.2rem;
    flex-shrink: 0;
    border-radius: var(--radius-xs);
    overflow: hidden;
  }

  .title {
    flex: 1;
    min-width: 0;
    font-size: var(--font-md);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .label {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
