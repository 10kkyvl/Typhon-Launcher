<script lang="ts">
  import { Heart } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Card from '../../lib/components/Card.svelte';
  import type { ShowcaseKind } from '../../lib/services/account';
  import type { ShowcaseBlock } from '../../lib/services/profile';
  import { SHOWCASE_TITLES, shortDate } from '../../lib/profile/view';
  import { navigate } from '../../lib/stores/router';

  let { blocks, onmanage }: { blocks: ShowcaseBlock[]; onmanage: () => void } = $props();

  const visible = $derived(blocks.filter((block) => block.games.length > 0));

  function title(kind: string): string {
    if (kind === 'favorites') return 'Любимые игры';
    return SHOWCASE_TITLES[kind as ShowcaseKind] ?? kind;
  }
</script>

{#snippet grid(block: ShowcaseBlock)}
  <div class="grid">
    {#each block.games as game (game.id)}
      <button class="tile" type="button" onclick={() => navigate('game', { id: game.id })}>
        <span class="cover">
          <Artwork src={game.cover} alt="" ratio="3 / 4" radius="var(--radius-md)" />
          {#if block.kind === 'favorites'}
            <span class="heart"><Heart size="1.4rem" strokeWidth={0} fill="currentColor" /></span>
          {/if}
        </span>
        <span class="caption">{game.title}</span>
        {#if block.kind === 'recently_completed' && game.statusAt}
          <span class="completed">Пройдена {shortDate(game.statusAt)}</span>
        {/if}
      </button>
    {/each}
  </div>
{/snippet}

{#each visible as block (block.kind)}
  {#if block.kind === 'favorites'}
    <Card title={title(block.kind)}>
      {#snippet action()}
        <button class="manage" type="button" onclick={onmanage}>Управлять избранным</button>
      {/snippet}
      {@render grid(block)}
    </Card>
  {:else}
    <Card title={title(block.kind)}>
      {@render grid(block)}
    </Card>
  {/if}
{/each}

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
    background: none;
    border: 0;
    padding: 0;
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

  .completed {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .manage {
    background: none;
    border: 0;
    padding: 0;
    color: var(--accent-text);
    font-size: var(--font-sm);
    font-weight: 500;
    cursor: pointer;
  }

  .manage:hover {
    text-decoration: underline;
  }
</style>
