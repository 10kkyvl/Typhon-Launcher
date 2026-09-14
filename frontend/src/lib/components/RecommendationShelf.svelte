<script lang="ts">
  import { RefreshCw, ThumbsDown } from '@lucide/svelte';
  import Button from './Button.svelte';
  import GameCard from './GameCard.svelte';
  import IconButton from './IconButton.svelte';
  import { msg } from '../i18n';
  import { genreLabel, recommendationReason, type Recommendation } from '../recommendations/display';
  import { gameArt } from '../stores/metadata';
  import { installedGames, runningGames } from '../stores/library';

  let { title, items, loading = false, emptyText, onrefresh, ondismiss, onplay }: {
    title: string;
    items: Recommendation[];
    loading?: boolean;
    emptyText: string;
    onrefresh?: () => void;
    ondismiss: (item: Recommendation) => void;
    onplay?: (libraryId: string) => void;
  } = $props();
</script>

<section class="recommendations" aria-label={title} aria-busy={loading}>
  <div class="heading">
    <h2>{title}</h2>
    {#if onrefresh}
      <Button onclick={onrefresh} disabled={loading}><RefreshCw size="1.4rem" />{msg('games.recommendationRefresh')}</Button>
    {/if}
  </div>
  {#if items.length > 0}
    <div class="picks">
      {#each items as item (item.game.id)}
        {@const installed = Boolean(item.libraryId && $installedGames.some((game) => game.id === item.libraryId))}
        <div class="pick">
          <GameCard id={item.libraryId || item.game.id} title={item.game.title}
            cover={item.game.coverUrl || $gameArt[item.game.id]?.cover || ''}
            meta={recommendationReason(item)} {installed}
            running={$runningGames.has(item.libraryId || '')}
            onplay={item.libraryId && onplay ? () => onplay?.(item.libraryId!) : undefined}>
            {#snippet footer()}
              <span class="genre">{item.game.genres?.slice(0, 2).map(genreLabel).join(' · ') || ''}</span>
              <IconButton label={msg('games.recommendationNotInterested')} size="sm" disabled={loading}
                onclick={() => ondismiss(item)}><ThumbsDown size="1.4rem" /></IconButton>
            {/snippet}
          </GameCard>
        </div>
      {/each}
    </div>
  {:else}
    <p class="empty" role="status">{loading ? msg('games.recommendationLoading') : emptyText}</p>
  {/if}
</section>

<style>
  .recommendations { margin-bottom: var(--space-8); }
  .heading { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); margin-bottom: var(--space-4); }
  h2 { font-size: var(--font-xl); }
  .picks { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: var(--space-5); }
  .pick { min-width: 0; }
  .genre, .empty { font-size: var(--font-sm); color: var(--text-3); }
  .genre { white-space: nowrap; text-overflow: ellipsis; overflow: hidden; }
  @media (max-width: 900px) { .picks { grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr)); } }
</style>
