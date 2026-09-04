<script lang="ts">
  import type { PlayedGame } from '../../lib/services/social';
  import { playtime, relativeDate } from '../../lib/utils/format';
  import GameRow from './GameRow.svelte';

  let { games }: { games: PlayedGame[] } = $props();

  function meta(game: PlayedGame): string {
    const played = relativeDate(game.lastPlayedAt);
    const seconds = game.playtimeSeconds ?? 0;
    return seconds > 0 ? `${playtime(seconds)} · ${played}` : played;
  }
</script>

<section class="group">
  <h3>Недавно играл</h3>
  <div class="list">
    {#each games as game (game.igdbId)}
      <GameRow {game} meta={meta(game)} />
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
