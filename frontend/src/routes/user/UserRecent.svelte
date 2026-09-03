<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import type { PlayedGame } from '../../lib/services/social';
  import { playtime, relativeDate } from '../../lib/utils/format';

  let { games }: { games: PlayedGame[] } = $props();
</script>

<section class="group">
  <h3>Недавно играл</h3>
  <div class="list">
    {#each games as game (game.igdbId)}
      <div class="row">
        <div class="thumb">
          <Artwork src={game.coverUrl} alt={game.title} ratio="3 / 4" />
        </div>
        <span class="title">{game.title}</span>
        <span class="meta">
          {#if game.playtimeSeconds != null}{playtime(game.playtimeSeconds)} · {/if}{relativeDate(game.lastPlayedAt)}
        </span>
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

  .meta {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
