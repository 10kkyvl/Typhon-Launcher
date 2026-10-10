<script lang="ts">
  import { Heart } from '@lucide/svelte';
  import Artwork from '../Artwork.svelte';
  import Card from '../Card.svelte';
  import { msg } from '../../i18n';
  import type { BlockGame } from '../../profile/layoutView';
  import { shortDate } from '../../profile/view';

  let { title, games, dated, hearts = false }: { title: string; games: BlockGame[]; dated: boolean; hearts?: boolean } = $props();
</script>

<Card title={title || undefined}>
  {#if games.length === 0}
    <p class="empty">{msg('profile.showcaseEmpty')}</p>
  {:else}
    <div class="grid">
      {#each games as game (game.key)}
        <button class="tile" type="button" onclick={game.open}>
          <span class="cover">
            <Artwork src={game.cover} alt="" label={game.title} ratio="3 / 4" radius="var(--radius-md)" />
            {#if hearts}
              <span class="heart"><Heart size="1.4rem" strokeWidth={0} fill="currentColor" /></span>
            {/if}
          </span>
          <span class="caption">{game.title}</span>
          {#if dated && game.completedAt}
            <span class="completed">{msg('social.completedOn', { date: shortDate(game.completedAt) })}</span>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
</Card>

<style>
  .empty {
    padding: var(--space-4) 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

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
    border-radius: var(--radius-md);
    box-shadow: inset 0 0 0 1px var(--border);
    transition:
      transform var(--dur-panel) var(--ease),
      box-shadow var(--dur-panel) var(--ease);
  }

  .tile:hover .cover,
  .tile:focus-visible .cover {
    transform: translateY(-0.3rem);
    box-shadow: var(--shadow-lift);
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

  .completed {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
