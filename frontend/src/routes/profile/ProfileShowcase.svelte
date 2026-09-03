<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import type { ShowcaseKind } from '../../lib/services/account';
  import type { ShowcaseBlock } from '../../lib/services/profile';
  import { SHOWCASE_HINTS, SHOWCASE_TITLES } from '../../lib/profile/view';
  import { navigate } from '../../lib/stores/router';

  let { blocks }: { blocks: ShowcaseBlock[] } = $props();

  const title = (kind: string) => SHOWCASE_TITLES[kind as ShowcaseKind] ?? kind;
  const hint = (kind: string) => SHOWCASE_HINTS[kind as ShowcaseKind] ?? '';
</script>

{#each blocks as block (block.kind)}
  <section class="group">
    <h3>{title(block.kind)}</h3>
    {#if block.games.length === 0}
      <p class="hint">{hint(block.kind)}</p>
    {:else}
      <div class="grid">
        {#each block.games as game (game.id)}
          <button class="tile" aria-label={game.title} onclick={() => navigate('game', { id: game.id })}>
            <Artwork src={game.cover} alt="" ratio="3 / 4" radius="var(--radius-md)" />
            <span class="caption">{game.title}</span>
          </button>
        {/each}
      </div>
    {/if}
  </section>
{/each}

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

  .hint {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(6, 1fr);
    gap: var(--space-3);
  }

  .tile {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
    background: none;
    border: 0;
    padding: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
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

  @media (max-width: 1200px) {
    .grid {
      grid-template-columns: repeat(3, 1fr);
    }
  }
</style>
