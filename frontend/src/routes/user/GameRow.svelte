<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import type { GameCard } from '../../lib/services/social';
  import { openGameByIGDB } from '../../lib/social/openGame';

  let { game, meta }: { game: GameCard; meta: string } = $props();
</script>

<button class="row" type="button" onclick={() => openGameByIGDB(game.igdbId, game.title)}>
  <span class="thumb">
    <Artwork src={game.coverUrl} alt={game.title} ratio="3 / 4" />
  </span>
  <span class="title">{game.title}</span>
  <span class="meta">{meta}</span>
</button>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    width: 100%;
    padding: 0.8rem;
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .thumb {
    display: block;
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
