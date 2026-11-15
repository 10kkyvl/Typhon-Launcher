<script lang="ts">
  import { Ellipsis, Heart, ThumbsDown, Undo2 } from '@lucide/svelte';
  import { mergeCatalogDisplay } from '../../lib/catalog/display';
  import Artwork from '../../lib/components/Artwork.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameCard, { type GameCardAction } from '../../lib/components/GameCard.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { playGame, stopGame } from '../../lib/services/library';
  import { canPlay } from '../../lib/library/launch';
  import type { CatalogGame } from '../../lib/services/sources';
  import { gameMenu, gameMenuButton } from '../../lib/stores/gameMenu';
  import { installedGames, libraryGames, runningGames } from '../../lib/stores/library';
  import { gameArt, gameInfo } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { catalogView } from '../../lib/stores/ui';
  import { msg } from '../../lib/i18n';
  import { libraryErrorText, type CatalogPageData } from './catalogPageData.svelte';

  // The games of the loaded pages, as a grid of cards or a list of rows; while the first page loads, its
  // skeleton; with none, why. The view switch's animation is the page's.
  let { data, viewSwitched, endViewSwitch }: {
    data: CatalogPageData;
    viewSwitched: boolean;
    endViewSwitch: (event: AnimationEvent) => void;
  } = $props();

  const installedByGame = $derived.by(() => {
    const map = new Map<string, string>();
    for (const game of $installedGames) {
      if (game.canonicalGameId) {
        map.set(game.canonicalGameId, game.id);
        for (const item of data.pages.items) if (item.aliasIds?.includes(game.canonicalGameId)) map.set(item.id,game.id);
      }
    }
    return map;
  });

  const playableIds = $derived(new Set($installedGames.filter(canPlay).map((game) => game.id)));

  const libraryByGame = $derived.by(() => {
    const map = new Map<string, string>();
    for (const game of $libraryGames) {
      if (game.canonicalGameId) {
        map.set(game.canonicalGameId, game.id);
        for (const item of data.pages.items) if (item.aliasIds?.includes(game.canonicalGameId)) map.set(item.id,game.id);
      }
    }
    return map;
  });

  const favoriteByLibraryId = $derived.by(() => {
    const map = new Map<string, boolean>();
    for (const game of $libraryGames) map.set(game.id, Boolean(game.favorite));
    return map;
  });

  function catalogMeta(game: CatalogGame) {
    const rating = game.rating != null && game.ratingCount != null && game.ratingCount > 0
      ? msg('games.catalogRating', { rating: Math.round(game.rating), count: game.ratingCount }) : '';
    return [game.developer, rating].filter(Boolean).join(' · ');
  }

  function listMeta(game: CatalogGame) { return [game.releaseYear, game.developer].filter(Boolean).join(" · "); }

  function cardActions(game: CatalogGame, title: string, libId: string | undefined, favorite: boolean): GameCardAction[] {
    const dismissed = data.isDismissed(game);
    const dismissAction: GameCardAction = {
      id: 'dismiss',
      label: dismissed ? msg('games.recommendationRestore') : msg('games.recommendationNotInterested'),
      icon: dismissed ? Undo2 : ThumbsDown,
      active: dismissed,
      pinned: dismissed,
      busy: data.preferenceBusy,
      onclick: () => void data.dismiss(game, !dismissed),
    };
    if (!libId) return [dismissAction];
    const moreAction: GameCardAction = {
      id: 'more',
      label: msg('games.installedMenuFor', { title }),
      icon: Ellipsis,
      menu: gameMenuButton(libId, `catalog-more-${game.id}`, $gameMenu),
    };
    const favoriteAction: GameCardAction = {
      id: 'favorite',
      label: favorite ? msg('games.actionFavoriteRemove') : msg('games.actionFavoriteAdd'),
      icon: Heart,
      active: favorite,
      filled: favorite,
      pinned: favorite,
      quiet: true,
      onclick: (event) => {
        event.stopPropagation();
        data.toggleFavorite(libId, favorite);
      },
    };
    return [moreAction, dismissAction, favoriteAction];
  }

  async function toggleRun(libraryId: string) {
    try {
      if ($runningGames.has(libraryId)) await stopGame(libraryId);
      else await playGame(libraryId);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
    }
  }
</script>

{#snippet installedStatus()}
  <StatusBadge kind="success" label={msg('games.gameInstalledWord')} plain />
{/snippet}

{#if data.pageFailed}
  <!-- Said by the notice above the list. -->
{:else if data.displayedItems.length === 0}
  {#if data.pages.loading}
    <Spinner label={msg('games.catalogLoadingLabel')} />
    <div class="catalog-skeleton" class:grid={$catalogView === 'grid'} class:loading-list={$catalogView !== 'grid'} aria-hidden="true">
      {#each Array(12) as _}
        <div class="loading-item">
          <div class="loading-skeleton loading-cover"></div>
          <div class="loading-copy">
            <div class="loading-skeleton loading-title"></div>
            <div class="loading-skeleton loading-meta"></div>
          </div>
        </div>
      {/each}
    </div>
  {:else if data.discoveryVisible && data.discovery.length > 0}
    <p class="muted" role="status">{msg('games.catalogAllInDiscovery')}</p>
  {:else if data.search.trim() || data.genre || data.platform || data.kind || data.compatOnly}
    <EmptyState level={2} title={msg('games.nothingFoundTitle')} description={msg('games.catalogNothingFoundDescription')} />
  {:else}
    <EmptyState level={2}
      title={msg('games.catalogEmptyTitle')}
      description={msg('games.catalogEmptyDescription')}
    />
  {/if}
{:else if $catalogView === 'grid'}
  <div class="grid" class:switched={viewSwitched} onanimationend={endViewSwitch} bind:this={data.results}>
    {#each data.displayedItems as game (game.id)}
      {@const shown = mergeCatalogDisplay(game, $gameInfo[game.id])}
      {@const libId = libraryByGame.get(game.id)}
      {@const isFav = libId ? Boolean(favoriteByLibraryId.get(libId)) : false}
      {@const isInstalled = installedByGame.has(game.id)}
      <div class="cell">
        <GameCard
          id={game.id}
          title={shown.title}
          cover={game.coverUrl || $gameArt[game.id]?.cover || ''}
          installed={isInstalled}
          running={$runningGames.has(installedByGame.get(game.id) ?? '')}
          meta={catalogMeta(shown)}
          compat={data.compatRelevant ? data.pages.compatByGame[game.id] : undefined}
          actions={cardActions(game, shown.title, libId, isFav)}
          footer={isInstalled ? installedStatus : undefined}
          onplay={playableIds.has(installedByGame.get(game.id) ?? '') ? () => toggleRun(installedByGame.get(game.id) ?? '') : undefined}
        />
      </div>
    {/each}
  </div>
{:else}
  <div class="list" class:switched={viewSwitched} onanimationend={endViewSwitch} bind:this={data.results}>
    {#each data.displayedItems as game (game.id)}
      {@const shown = mergeCatalogDisplay(game, $gameInfo[game.id])}
      <div class="list-entry">
      <button
        class="list-row"
        onclick={() => navigate('game', { id: game.id })}
      >
        <div class="list-thumb" aria-hidden="true">
          <Artwork src={game.coverUrl || $gameArt[game.id]?.cover || ''} alt="" label={shown.title} radius="var(--radius-xs)" />
        </div>
        <span class="list-title">{shown.title}</span>
        <span class="list-meta">{listMeta(shown)}</span>
        {#if installedByGame.has(game.id)}
          <span class="list-meta right">{@render installedStatus()}</span>
        {/if}
      </button>
      <span class="list-action" class:pinned={data.isDismissed(game)}>
        <IconButton label={data.isDismissed(game) ? msg('games.recommendationRestore') : msg('games.recommendationNotInterested')}
          lit={data.isDismissed(game)} disabled={data.preferenceBusy} onclick={() => void data.dismiss(game, !data.isDismissed(game))}>
          {#if data.isDismissed(game)}<Undo2 size="1.4rem" />{:else}<ThumbsDown size="1.4rem" />{/if}
        </IconButton>
      </span></div>
    {/each}
  </div>
{/if}

<style>
  .list-entry { display: flex; align-items: center; }
  .list-entry .list-row { flex: 1; min-width: 0; }
  .list-action { display: inline-flex; opacity: 0; pointer-events: none; transition: opacity var(--dur) var(--ease); }
  .list-entry:hover .list-action, .list-entry:focus-within .list-action, .list-action.pinned { opacity: 1; pointer-events: auto; }

  .catalog-skeleton { margin-top: var(--space-4); }
  .loading-item { min-width: 0; }
  .loading-cover { aspect-ratio: 3 / 4; border-radius: var(--radius-md); }
  .loading-copy { margin-top: 0.9rem; }
  .loading-title { height: 1.6rem; width: 75%; }
  .loading-meta { height: 1.2rem; width: 45%; margin-top: 0.6rem; }
  .loading-list { display: grid; gap: var(--space-3); }
  .loading-list .loading-item { display: flex; align-items: center; gap: var(--space-3); }
  .loading-list .loading-cover { width: 5rem; flex-shrink: 0; }
  .loading-list .loading-copy { width: min(32rem, 60%); margin-top: 0; }

  .cell {
    min-width: 0;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--space-6) var(--space-5);
  }

  .grid:not(.catalog-skeleton),
  .list {
    animation: media-in var(--dur-panel) var(--ease);
  }

  .grid.switched:not(.catalog-skeleton),
  .list.switched {
    animation: rise-in var(--dur-panel) var(--ease);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .list-row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 0.8rem;
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .list-entry + .list-entry {
    border-top: 1px solid var(--border);
  }

  .list-row:hover {
    background: var(--hover);
  }

  .list-thumb {
    width: 3.2rem;
    height: 4.2rem;
    flex-shrink: 0;
    border-radius: var(--radius-xs);
    overflow: hidden;
  }

  .list-title {
    flex: 1;
    min-width: 0;
    font-size: var(--font-md);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .list-meta {
    font-size: var(--font-sm);
    color: var(--text-3);
    white-space: nowrap;
  }

  .list-meta.right {
    min-width: 10rem;
    text-align: right;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  @media (min-width: 2200px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    }
  }

  @media (max-width: 1400px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(14.5rem, 1fr));
    }
  }
</style>
