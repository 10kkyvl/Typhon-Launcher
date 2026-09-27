<script lang="ts">
  import Card from '../../lib/components/Card.svelte';
  import type { CommonGames } from '../../lib/services/social';
  import { commonGameLabel, commonGamesTitle } from '../../lib/social/view';
  import { msg } from '../../lib/i18n';
  import GameRow from './GameRow.svelte';

  let { common, name }: { common: CommonGames; name: string } = $props();

  const rows = $derived(common.games.slice(0, 6));
</script>

<Card title={msg('social.userCommonTitle')}>
  <p class="count">{commonGamesTitle(common.count)}</p>
  <div class="list">
    {#each rows as game (game.igdbId)}
      <GameRow {game} meta={commonGameLabel(game.viewerOwned, game.targetOwned, name)} />
    {/each}
  </div>
</Card>

<style>
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
