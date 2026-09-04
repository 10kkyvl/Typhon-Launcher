<script lang="ts">
  import { ArrowDownUp, ChevronDown, Download, EllipsisVertical, Heart, LayoutGrid, List } from '@lucide/svelte';
  import { onDestroy, onMount } from 'svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { playGame, setFavorite, stopGame } from '../../lib/services/library';
  import { getGenreFacets, queryCatalogGames, type CatalogGame, type GenreFacet } from '../../lib/services/sources';
  import { openGameMenu } from '../../lib/stores/gameMenu';
  import { installedGames, libraryGames, runningGames } from '../../lib/stores/library';
  import { gameArt, gameInfo, loadArt, requestArt } from '../../lib/stores/metadata';
  import { currentRouteKey, navigate, recallRoute, stashRoute } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { catalogView } from '../../lib/stores/ui';
  import { inview } from '../../lib/utils/inview';
  import { msg } from '../../lib/i18n';

  type Sort = 'title' | 'year' | 'added';

  const pageSize = 60;
  const allGenres = msg('games.filterAll');
  const sortLabels: Record<Sort, string> = {
    title: msg('games.sortAlpha'),
    year: msg('games.catalogSortYear'),
    added: msg('games.catalogSortAdded'),
  };

  interface Snapshot {
    search: string;
    genre: string;
    sort: Sort;
    items: CatalogGame[];
    total: number;
    page: number;
    failed: boolean;
  }

  const routeKey = currentRouteKey();
  const restored = recallRoute<Snapshot>(routeKey, 'catalog');

  let search = $state(restored?.search ?? '');
  let genre = $state(restored?.genre ?? '');
  let sort = $state<Sort>(restored?.sort ?? 'title');
  let items = $state<CatalogGame[]>(restored?.items ?? []);
  let total = $state(restored?.total ?? 0);
  let page = $state(restored?.page ?? 0);
  let loading = $state(!restored);
  let appending = $state(false);
  let failed = $state(restored?.failed ?? false);
  let facets = $state<GenreFacet[]>([]);

  let token = 0;
  let debounce: ReturnType<typeof setTimeout> | undefined;
  const seen = new Set<string>();

  function see(id: string) {
    seen.add(id);
    requestArt([id]);
  }

  onDestroy(() => {
    clearTimeout(debounce);
    stashRoute(routeKey, 'catalog', {
      search,
      genre,
      sort,
      items: [...items],
      total,
      page,
      failed,
    } satisfies Snapshot);
  });

  const installedByGame = $derived.by(() => {
    const map = new Map<string, string>();
    for (const game of $installedGames) {
      if (game.canonicalGameId) map.set(game.canonicalGameId, game.id);
    }
    return map;
  });

  const libraryByGame = $derived.by(() => {
    const map = new Map<string, string>();
    for (const game of $libraryGames) {
      if (game.canonicalGameId) map.set(game.canonicalGameId, game.id);
    }
    return map;
  });

  const favoriteByLibraryId = $derived.by(() => {
    const map = new Map<string, boolean>();
    for (const game of $libraryGames) map.set(game.id, Boolean(game.favorite));
    return map;
  });

  const chips = $derived([allGenres, ...facets.filter((f) => f.count > 0).map((f) => f.label)]);

  async function loadFacets() {
    try {
      facets = await getGenreFacets();
    } catch {
      facets = [];
    }
  }

  async function fetchPage(next: number) {
    const current = ++token;
    loading = true;
    appending = next > 1;
    try {
      const result = await queryCatalogGames({ search, genre, sort, page: next, pageSize });
      if (current !== token) return;
      items = next === 1 ? result.items : [...items, ...result.items];
      total = result.total;
      page = result.page;
      failed = false;
      loadArt(result.items.map((game) => game.id));
    } catch {
      if (current !== token) return;
      if (next === 1) {
        items = [];
        total = 0;
      }
      failed = true;
    } finally {
      if (current === token) {
        loading = false;
        appending = false;
      }
    }
  }

  function reload() {
    page = 0;
    seen.clear();
    fetchPage(1);
  }

  function onSearch() {
    clearTimeout(debounce);
    debounce = setTimeout(reload, 250);
  }

  function onSort(value: Sort) {
    if (value === sort) return;
    sort = value;
    reload();
  }

  function onGenre(label: string) {
    const value = label === allGenres ? '' : label;
    if (value === genre) return;
    genre = value;
    reload();
  }

  onMount(() => {
    loadFacets();
    if (restored) requestArt(items.map((game) => game.id));
    else reload();
  });

  onMount(() => {
    const timer = setInterval(() => {
      const missing = [...seen].filter((id) => !$gameArt[id]?.cover);
      if (missing.length > 0) requestArt(missing);
    }, 20000);
    return () => clearInterval(timer);
  });

  function listMeta(game: CatalogGame) {
    const bits: string[] = [];
    if (game.releaseYear) bits.push(String(game.releaseYear));
    if (game.developer) bits.push(game.developer);
    return bits.join(' · ');
  }

  async function toggleRun(libraryId: string) {
    try {
      if ($runningGames.has(libraryId)) await stopGame(libraryId);
      else await playGame(libraryId);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : msg('games.errorPlayFailed'), 'danger');
    }
  }

  async function toggleFavorite(libraryId: string, current: boolean) {
    try {
      await setFavorite(libraryId, !current);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : msg('games.errorFavoriteFailed'), 'danger');
    }
  }
</script>

<Card surface="panel">
  <PageHeader title={msg('games.allGamesTitle')} />

  <div class="search-row">
    <SearchInput bind:value={search} placeholder={msg('games.catalogSearchPlaceholder')} loading={loading && !appending} oninput={onSearch} />
  </div>

  <div class="filter-row">
    <div class="chips">
      {#each chips as label (label)}
        <Chip variant="outline" selected={(label === allGenres ? '' : label) === genre} onclick={() => onGenre(label)}>
          {label}
        </Chip>
      {/each}
    </div>
    <div class="controls">
      <DropdownMenu
        items={[
          { id: 'title', label: sortLabels.title },
          { id: 'year', label: sortLabels.year },
          { id: 'added', label: sortLabels.added },
        ]}
        onselect={(id) => onSort(id as Sort)}
      >
        {#snippet trigger({ open, toggle })}
          <button class="sort" class:open onclick={toggle}>
            <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
            {sortLabels[sort]}
            <ChevronDown size="1.4rem" strokeWidth={1.8} />
          </button>
        {/snippet}
      </DropdownMenu>
      <SegmentedControl
        bind:value={$catalogView}
        options={[
          { id: 'grid', label: msg('games.viewGrid') },
          { id: 'list', label: msg('games.viewList') },
        ]}
      >
        {#snippet item(option)}
          {#if option.id === 'grid'}
            <LayoutGrid size="1.6rem" strokeWidth={1.8} />
          {:else}
            <List size="1.6rem" strokeWidth={1.8} />
          {/if}
        {/snippet}
      </SegmentedControl>
    </div>
  </div>

  {#if items.length === 0}
    {#if loading}
      <p class="muted">{msg('games.catalogLoadingLabel')}</p>
    {:else if failed}
      <EmptyState
        title={msg('games.catalogUnavailableTitle')}
        description={msg('games.catalogUnavailableDescription')}
      />
    {:else if search.trim() || genre}
      <EmptyState title={msg('games.nothingFoundTitle')} description={msg('games.catalogNothingFoundDescription')} />
    {:else}
      <EmptyState
        title={msg('games.catalogEmptyTitle')}
        description={msg('games.catalogEmptyDescription')}
      />
    {/if}
  {:else if $catalogView === 'grid'}
    <div class="grid">
      {#each items as game (game.id)}
        {@const shown = $gameInfo[game.id] ?? game}
        {@const libId = libraryByGame.get(game.id)}
        {@const isFav = libId ? favoriteByLibraryId.get(libId) : false}
        {@const isInstalled = installedByGame.has(game.id)}
        <div class="cell" use:inview={() => see(game.id)}>
          <GameCard
            id={game.id}
            title={shown.title}
            cover={$gameArt[game.id]?.cover ?? ''}
            installed={isInstalled}
            running={$runningGames.has(installedByGame.get(game.id) ?? '')}
            meta={shown.developer ?? ''}
            onplay={() => toggleRun(installedByGame.get(game.id) ?? '')}
          >
            {#snippet footer()}
              <span class="status" class:on={isInstalled}>
                {#if isInstalled}
                  <span class="dot"></span>{msg('games.gameInstalledWord')}
                {:else}
                  <Download size="1.3rem" strokeWidth={1.8} />{msg('games.gameNotInstalledWord')}
                {/if}
              </span>
              {#if libId}
                <div class="actions">
                  <IconButton
                    label={isFav ? msg('games.actionFavoriteRemove') : msg('games.actionFavoriteAdd')}
                    size="sm"
                    active={isFav}
                    onclick={(event) => {
                      event.stopPropagation();
                      toggleFavorite(libId, Boolean(isFav));
                    }}
                  >
                    <Heart size="1.5rem" strokeWidth={1.8} fill={isFav ? 'currentColor' : 'none'} />
                  </IconButton>
                  <IconButton
                    label={msg('games.moreLabel')}
                    size="sm"
                    onclick={(event) => openGameMenu(event, libId)}
                  >
                    <EllipsisVertical size="1.5rem" strokeWidth={1.8} />
                  </IconButton>
                </div>
              {/if}
            {/snippet}
          </GameCard>
        </div>
      {/each}
    </div>
  {:else}
    <div class="list">
      {#each items as game (game.id)}
        {@const shown = $gameInfo[game.id] ?? game}
        <button
          class="list-row"
          use:inview={() => see(game.id)}
          onclick={() => navigate('game', { id: game.id })}
        >
          <div class="list-thumb">
            <Artwork src={$gameArt[game.id]?.cover ?? ''} alt={shown.title} radius="var(--radius-xs)" />
          </div>
          <span class="list-title">{shown.title}</span>
          <span class="list-meta">{listMeta(shown)}</span>
          <span class="list-meta right">
            {installedByGame.has(game.id) ? msg('games.gameInstalledWord') : msg('games.gameNotInstalledWord')}
          </span>
        </button>
      {/each}
    </div>
  {/if}

  {#if items.length > 0 && items.length < total}
    <div class="more">
      <Button onclick={() => fetchPage(page + 1)} disabled={loading}>
        {appending ? msg('games.catalogLoadingMore') : msg('games.catalogShowMore')}
      </Button>
      <span class="muted">{msg('games.catalogShownOf', { shown: items.length, total })}</span>
    </div>
  {/if}
</Card>

<style>
  .search-row {
    max-width: 46rem;
    margin-bottom: var(--space-4);
  }

  .filter-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-6);
    flex-wrap: wrap;
  }

  .chips {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-shrink: 0;
  }

  .sort {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: var(--control-sm);
    padding: 0 1.1rem;
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .sort:hover,
  .sort.open {
    background: var(--hover);
    color: var(--text);
  }

  .cell {
    min-width: 0;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--space-6) var(--space-5);
  }

  .status {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .status.on {
    color: var(--text-2);
  }

  .dot {
    width: 0.7rem;
    height: 0.7rem;
    flex-shrink: 0;
    border-radius: 50%;
    background: var(--success);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    flex-shrink: 0;
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

  .list-row + .list-row {
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
