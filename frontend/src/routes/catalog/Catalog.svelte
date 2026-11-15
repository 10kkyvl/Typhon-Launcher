<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { Ellipsis, Heart, ThumbsDown, Undo2 } from '@lucide/svelte';
  import { mergeCatalogDisplay } from '../../lib/catalog/display';
  import { get } from 'svelte/store';
  import Notice from '../../lib/components/Notice.svelte';
  import RecommendationShelf from '../../lib/components/RecommendationShelf.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameCard, { type GameCardAction } from '../../lib/components/GameCard.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
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
  import { createCatalogPageData, libraryErrorText } from './catalogPageData.svelte';
  import CatalogToolbar from './CatalogToolbar.svelte';

  // Created first: its effects are the page's, and run before the view switch's below.
  const data = createCatalogPageData();

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

  let lastView = get(catalogView);
  let viewSwitched = $state(false);

  $effect(() => {
    if ($catalogView === lastView) return;
    lastView = $catalogView;
    viewSwitched = true;
  });

  function endViewSwitch(event: AnimationEvent) {
    if (event.target === event.currentTarget) viewSwitched = false;
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

<Page>
  <PageHeader
    title={msg('games.allGamesTitle')}
    count={data.pages.total > 0 ? msg('games.catalogTotalCount', { count: data.pages.total }) : undefined}
  />

  <CatalogToolbar {data} />

  {#if data.pages.offline || data.pageFailed || data.pageRetrying}
    <div class="page-note">
      <Notice
        tone={data.pageFailed ? 'danger' : 'warning'}
        title={data.pageFailed ? msg('games.catalogUnavailableTitle') : undefined}
        level={data.pageFailed ? 2 : undefined}
        bind:retrying={data.pageRetrying}
        retry={{ label: msg('games.catalogRetry'), run: data.retryFirstPage, within: () => data.results }}
      >
        {#if !data.pageFailed}
          {msg('games.catalogOffline')}
        {:else if data.pages.backendOutdated}
          {msg('games.catalogBackendOutdated')}
        {:else}
          {msg('games.catalogUnavailableDescription')}
        {/if}
      </Notice>
    </div>
  {:else if data.pages.incomplete}<details class="muted"><summary>{msg('games.catalogCoverage')}</summary><p>{msg('games.catalogIncomplete')}</p></details>{/if}

  {#if data.lastDismissed}
    <div class="page-note">
      <Notice tone="success">
        {msg('games.recommendationDismissed', { title: data.lastDismissed.title })}
        {#snippet action()}<Button size="sm" onclick={data.undoDismissal} busy={data.undoing} disabled={data.preferenceBusy && !data.undoing}>{msg('games.recommendationUndo')}</Button>{/snippet}
      </Notice>
    </div>
  {/if}
  <span bind:this={data.dismissalEnd} hidden></span>
  {#if data.personalizationFallback}<div class="page-note"><Notice tone="info" compact>{msg('games.recommendationFallback')}</Notice></div>{/if}
  {#if data.discoveryStale}<div class="page-note"><Notice tone="warning" compact>{msg('games.recommendationRefreshFailed')}</Notice></div>{/if}
  {#if data.profile.confidence < 1 && (data.sort === 'auto' || data.sort === 'for-you')}<p class="muted profile-hint">{msg('games.recommendationNoHistory')}</p>{/if}
  {#if data.discoveryVisible}
    <RecommendationShelf title={msg('games.discoveryTitle')} items={data.discovery} loading={data.discoveryLoading || data.preferenceBusy}
      emptyText={msg('games.recommendationEmpty')} onrefresh={data.refreshPicks} ondismiss={(item) => void data.dismiss(item.game)} />
  {/if}

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

  {#if data.pages.items.length > 0}
    {#if data.autoLoad}<div class="sentinel" bind:this={data.sentinel} aria-hidden="true"></div>{/if}
    <div class="more" bind:this={data.moreSlot}>
      {#if data.pages.failed || data.moreRetrying}
        <!-- A server too old for this sorting refuses every page the same way: no Retry for it. -->
        <Notice
          tone="danger"
          compact
          tabindex={data.pages.backendOutdated ? -1 : undefined}
          data-more-refused={data.pages.backendOutdated || undefined}
          bind:retrying={data.moreRetrying}
          retry={data.pages.backendOutdated ? undefined : { label: msg('games.catalogRetry'), run: data.retryNextPage, focus: data.firstFresh }}
        >
          {data.pages.backendOutdated ? msg('games.catalogBackendOutdated') : msg('games.catalogMoreFailed')}
        </Notice>
      {:else if data.showMoreButton}
        <Button onclick={data.showMore} busy={data.pages.loading}>{msg('games.catalogShowMore')}</Button>
      {:else if data.hasMore && data.pages.appending}
        <Spinner label={msg('games.catalogLoadingMore')} />
      {/if}
      <span class="muted">{msg('games.catalogShownOf', { shown: data.pages.items.length, total: data.pages.total })}</span>
    </div>
  {/if}
</Page>

<style>
  .page-note { margin: var(--space-4) 0; }
  .profile-hint { margin-bottom: var(--space-5); }
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

  .sentinel { height: 1px; }

  .more {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    justify-content: center;
    padding: var(--space-8) 0 var(--space-4);
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
