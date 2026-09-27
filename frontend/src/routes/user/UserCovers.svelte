<script lang="ts">
  import { Heart } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Card from '../../lib/components/Card.svelte';
  import type { GameCard } from '../../lib/services/social';
  import { openGameByIGDB } from '../../lib/social/openGame';

  let {
    title,
    games,
    hearts = false,
  }: { title: string; games: GameCard[]; hearts?: boolean } = $props();

  const shown = $derived(games.slice(0, 6));
</script>

{#if shown.length > 0}
  <Card {title}>
    <div class="grid">
      {#each shown as game (game.igdbId)}
        <button class="tile" type="button" onclick={() => openGameByIGDB(game.igdbId, game.title)}>
          <span class="cover">
            <Artwork src={game.coverUrl} alt={game.title} ratio="3 / 4" radius="var(--radius-md)" />
            {#if hearts}
              <span class="heart"><Heart size="1.4rem" strokeWidth={0} fill="currentColor" /></span>
            {/if}
          </span>
          <span class="caption">{game.title}</span>
        </button>
      {/each}
    </div>
  </Card>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
    gap: var(--space-3);
  }

  .tile {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .cover {
    position: relative;
    display: block;
  }

  .heart {
    position: absolute;
    left: 0.7rem;
    bottom: 0.7rem;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.6rem;
    height: 2.6rem;
    border-radius: 50%;
    background: rgba(5, 8, 12, 0.6);
    color: var(--danger);
  }

  .caption {
    width: 100%;
    font-size: var(--font-sm);
    font-weight: 500;
    line-height: 1.3;
    color: var(--text);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>
