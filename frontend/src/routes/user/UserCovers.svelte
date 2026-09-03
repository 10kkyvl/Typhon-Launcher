<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import type { GameCard } from '../../lib/services/social';

  let { title, games }: { title: string; games: GameCard[] } = $props();

  const shown = $derived(games.slice(0, 6));
</script>

{#if shown.length > 0}
  <section class="group">
    <h3>{title}</h3>
    <div class="grid">
      {#each shown as game (game.igdbId)}
        <div class="tile">
          <Artwork src={game.coverUrl} alt={game.title} ratio="3 / 4" radius="var(--radius-md)" />
          <span class="caption">{game.title}</span>
        </div>
      {/each}
    </div>
  </section>
{/if}

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

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(12rem, 1fr));
    gap: var(--space-3);
  }

  .tile {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
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
