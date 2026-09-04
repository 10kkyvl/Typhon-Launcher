<script lang="ts">
  import type { CommonGames } from '../../lib/services/social';
  import { commonGameLabel, commonGamesTitle } from '../../lib/social/view';
  import GameRow from './GameRow.svelte';

  let { common, name }: { common: CommonGames; name: string } = $props();

  const rows = $derived(common.games.slice(0, 6));
</script>

<section class="group">
  <h3>Играете оба</h3>
  <p class="count">{commonGamesTitle(common.count)}</p>
  <div class="list">
    {#each rows as game (game.igdbId)}
      <GameRow {game} meta={commonGameLabel(game.viewerOwned, game.targetOwned, name)} />
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

  .list :global(.row + .row) {
    border-top: 1px solid var(--border);
  }
</style>
