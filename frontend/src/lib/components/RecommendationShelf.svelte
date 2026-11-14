<script lang="ts">
  import { RefreshCw, ThumbsDown } from '@lucide/svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import GameCard from './GameCard.svelte';
  import Spinner from './Spinner.svelte';
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
      <Button onclick={onrefresh} busy={loading}>{#if !loading}<RefreshCw size="1.4rem" />{/if}{msg('games.recommendationRefresh')}</Button>
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
            actions={[{
              id: 'dismiss',
              label: msg('games.recommendationNotInterested'),
              icon: ThumbsDown,
              busy: loading,
              onclick: () => ondismiss(item),
            }]}
            onplay={item.libraryId && onplay ? () => onplay?.(item.libraryId!) : undefined}>
            {#snippet footer()}
              <span class="genre">{item.game.genres?.slice(0, 2).map(genreLabel).join(' · ') || ''}</span>
            {/snippet}
          </GameCard>
        </div>
      {/each}
    </div>
  {:else}
    <!-- One region from the search to its answer: a region inserted together with its words may not
         be read. The ring is the region's picture; its words are the loading line. -->
    <div role="status">
      {#if loading}
        <span class="loading"><Spinner size="sm" />{msg('games.recommendationLoading')}</span>
      {:else}
        <EmptyState size="inline" title={emptyText} />
      {/if}
    </div>
  {/if}
</section>

<style>
  .recommendations { margin-bottom: var(--space-8); }
  .heading { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); margin-bottom: var(--space-4); }
  h2 { font-size: var(--font-xl); }
  .picks { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: var(--space-5); }
  .pick { min-width: 0; }
  .genre { font-size: var(--font-sm); color: var(--text-3); }
  .loading { display: inline-flex; align-items: center; gap: var(--space-2); font-size: var(--font-sm); color: var(--text-3); }
  .genre { white-space: nowrap; text-overflow: ellipsis; overflow: hidden; }
  @media (max-width: 900px) { .picks { grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr)); } }
</style>
