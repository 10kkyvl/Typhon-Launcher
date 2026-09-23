<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { Search, ChevronLeft, ChevronRight, RotateCw, X, ArrowLeft } from '@lucide/svelte';
  import Artwork from '../../components/Artwork.svelte';
  import { errorCode, t, type MessageKey, type Params } from '../../i18n';
  import { type CatalogGame, type CatalogPage, type GenreFacet } from '../../services/sources';
  import { CatalogController, type CatalogLoadResult } from '../catalogController';
  import type { RequestText } from '../contracts';

  import { bigpictureCatalog } from '../../i18n/catalog/en/bigpictureCatalog';

  let { onback, ongame, requestText }: {
    onback: () => void;
    ongame: (id: string) => void;
    requestText: RequestText;
  } = $props();

  type CatalogMessage = keyof typeof bigpictureCatalog;
  const bp = (key: CatalogMessage, params?: Params) => $t(key as MessageKey, params);
  const pageSize = 24;
  const sortOptions = [
    { id: 'popular', label: 'bp.catalog.sortPopular' },
    { id: 'rating', label: 'bp.catalog.sortRating' },
    { id: 'year', label: 'bp.catalog.sortYear' },
    { id: 'title', label: 'bp.catalog.sortTitle' },
  ] as const;

  let items = $state<CatalogGame[]>([]);
  let facets = $state<GenreFacet[]>([]);
  let page = $state(0);
  let total = $state(0);
  let query = $state('');
  let genre = $state('');
  let sort = $state<(typeof sortOptions)[number]['id']>('popular');
  let loading = $state(false);
  let offline = $state(false);
  let pageError = $state('');
  const catalog = new CatalogController();
  const shownFacets = $derived(facets.filter((item) => item.count > 0));
  const pageCount = $derived(Math.max(1, Math.ceil(total / pageSize)));
  const canGoNext = $derived(!loading && page > 0 && page < pageCount);
  const canGoPrevious = $derived(!loading && page > 1);

  onMount(() => {
    void loadPage(1);
  });

  onDestroy(() => catalog.invalidate());

  async function loadPage(nextPage: number) {
    await runCatalogRequest(catalog.load({ search: query, genre, sort }, nextPage, pageSize));
  }

  async function retryCatalog() {
    await runCatalogRequest(catalog.retry());
  }

  async function runCatalogRequest(request: Promise<CatalogLoadResult>) {
    loading = true;
    pageError = '';
    const response = await request;
    if (response.kind === 'stale') return;
    if (response.kind === 'error') {
      pageError = bp('bp.catalog.loadFailed');
      offline = false;
      if (response.catalogChanged || errorCode(response.error) === 'catalog.changed') {
        items = [];
        facets = [];
        page = 0;
        total = 0;
      }
    } else {
      applyPage(response.page, response.firstPage);
    }
    loading = false;
  }

  function applyPage(result: CatalogPage, firstPage: boolean) {
    items = result.items ?? [];
    facets = firstPage ? result.facets ?? [] : facets;
    total = result.total ?? 0;
    page = result.page || 1;
    offline = result.offline ?? false;
  }

  async function openSearch() {
    const result = await requestText({
      title: bp('bp.catalog.searchTitle'),
      initialValue: query,
      maxLength: 160,
    });
    if (result === null) return;
    query = result.trim();
    resetQueryPage();
    await loadPage(1);
  }

  async function chooseGenre(value: string) {
    if (genre === value) return;
    genre = value;
    resetQueryPage();
    await loadPage(1);
  }

  async function chooseSort(value: (typeof sortOptions)[number]['id']) {
    if (sort === value) return;
    sort = value;
    resetQueryPage();
    await loadPage(1);
  }

  function resetQueryPage() {
    items = [];
    facets = [];
    page = 0;
    total = 0;
    offline = false;
  }

  function clearFilters() {
    query = '';
    genre = '';
    resetQueryPage();
    void loadPage(1);
  }

  function focusId(value: string) {
    return encodeURIComponent(value);
  }
</script>

<section class="bp-catalog" aria-label={bp('bp.catalog.title')}>
  <header class="bp-page-header">
    <div class="heading-copy">
      <button class="bp-button secondary back-button" data-bp-focus="catalog:back" onclick={onback}>
        <ArrowLeft size="1.8rem" />{bp('bp.catalog.back')}
      </button>
      <span class="eyebrow">{bp('bp.catalog.eyebrow')}</span>
      <h1>{bp('bp.catalog.title')}</h1>
      <p>{bp('bp.catalog.subtitle')}</p>
    </div>
    <div class="head-actions">
      <button class="bp-button search-button" data-bp-focus="catalog:search" data-bp-default onclick={openSearch}>
        <Search size="2rem" strokeWidth={2} />
        <span>{query || bp('bp.catalog.search')}</span>
      </button>
      {#if query || genre}
        <button class="bp-button secondary clear-button" data-bp-focus="catalog:clear" onclick={clearFilters}>
          <X size="1.8rem" />{bp('bp.catalog.clearFilters')}
        </button>
      {/if}
    </div>
  </header>

  <div class="filter-panel">
    <div class="filter-caption">
      <span>{bp('bp.catalog.genres')}</span>
      <span class="count">{total > 0 ? bp('bp.catalog.results', { count: total }) : ''}</span>
    </div>
    <div class="filter-strip" aria-label={bp('bp.catalog.genres')}>
      <button class="filter-chip" class:selected={!genre} data-bp-focus="catalog:genre:all" onclick={() => void chooseGenre('')}>
        {bp('bp.catalog.allGenres')}
      </button>
      {#each shownFacets as facet (facet.label)}
        <button class="filter-chip" class:selected={genre === facet.label} data-bp-focus={`catalog:genre:${focusId(facet.label)}`} onclick={() => void chooseGenre(facet.label)}>
          {facet.label}<span>{facet.count}</span>
        </button>
      {/each}
    </div>
    <div class="sort-strip" aria-label={bp('bp.catalog.sort')}>
      <span class="sort-label">{bp('bp.catalog.sort')}</span>
      {#each sortOptions as option (option.id)}
        <button class="sort-button" class:selected={sort === option.id} data-bp-focus={`catalog:sort:${option.id}`} onclick={() => void chooseSort(option.id)}>
          {bp(option.label)}
        </button>
      {/each}
    </div>
  </div>

  {#if offline}
    <div class="notice" role="status">{bp('bp.catalog.offline')}</div>
  {/if}
  {#if pageError}
    <div class="notice error" role="alert">
      <span>{pageError}</span>
      <button class="bp-button secondary compact" data-bp-focus="catalog:retry" onclick={() => void retryCatalog()}>
        <RotateCw size="1.7rem" />{bp('bp.catalog.retry')}
      </button>
    </div>
  {/if}

  {#if loading && items.length === 0}
    <div class="catalog-grid loading-grid" aria-hidden="true">
      {#each Array(12) as _, i (`skeleton-${i}`)}
        <div class="skeleton-card"><div class="skeleton-cover"></div><div class="skeleton-line"></div></div>
      {/each}
    </div>
  {:else if !loading && items.length === 0 && !pageError}
    <div class="bp-empty">
      <span class="empty-mark">⌕</span>
      <h2>{query || genre ? bp('bp.catalog.emptySearchTitle') : bp('bp.catalog.emptyTitle')}</h2>
      <p>{query || genre ? bp('bp.catalog.emptySearchText') : bp('bp.catalog.emptyText')}</p>
      {#if query || genre}
        <button class="bp-button secondary" data-bp-focus="catalog:empty-clear" onclick={clearFilters}>{bp('bp.catalog.clearFilters')}</button>
      {/if}
    </div>
  {:else}
    <div class="catalog-grid" aria-label={bp('bp.catalog.title')} aria-busy={loading}>
      {#each items as game (game.id)}
        <button class="catalog-card" data-bp-focus={`catalog:game:${focusId(game.id)}`} onclick={() => ongame(game.id)}>
          <span class="cover"><Artwork src={game.coverUrl ?? ''} alt={game.title} label={game.title} ratio="3 / 4" radius="1.2rem" /></span>
          <span class="game-copy">
            <strong>{game.title}</strong>
            <span class="meta">{[game.releaseYear, game.developer].filter(Boolean).join(' · ') || bp('bp.catalog.noDetails')}</span>
          </span>
        </button>
      {/each}
    </div>
  {/if}

  {#if page > 0 && total > 0}
    <nav class="pagination" aria-label={bp('bp.catalog.pagination')}>
      <button class="bp-button secondary page-button" data-bp-focus="catalog:previous" disabled={!canGoPrevious} onclick={() => void loadPage(page - 1)}>
        <ChevronLeft size="2rem" />{bp('bp.catalog.previous')}
      </button>
      <span class="page-count">{bp('bp.catalog.pageCount', { current: page, total: pageCount })}</span>
      <button class="bp-button secondary page-button" data-bp-focus="catalog:next" disabled={!canGoNext} onclick={() => void loadPage(page + 1)}>
        {bp('bp.catalog.next')}<ChevronRight size="2rem" />
      </button>
    </nav>
  {/if}
</section>

<style>
  .bp-catalog { display: flex; flex-direction: column; gap: 2.4rem; max-width: 160rem; margin: 0 auto; padding: 3.2rem clamp(2rem, 4vw, 6rem) 5rem; }
  .bp-page-header { display: flex; align-items: end; justify-content: space-between; gap: 2.4rem; }
  .heading-copy { min-width: 0; }
  .eyebrow { display: block; margin-bottom: .8rem; color: var(--accent); font-size: 1.4rem; font-weight: 750; letter-spacing: .14em; text-transform: uppercase; }
  h1 { margin: 0; font-size: clamp(3.2rem, 4vw, 5rem); line-height: 1.04; letter-spacing: -.035em; }
  .heading-copy p { margin: 1rem 0 0; color: var(--text-2); font-size: 1.8rem; }
  .head-actions { display: flex; align-items: center; gap: 1rem; }
  .bp-button { min-height: 5.8rem; display: inline-flex; align-items: center; justify-content: center; gap: 1rem; padding: 0 2rem; border: 1px solid transparent; border-radius: 1.2rem; background: var(--accent); color: var(--accent-on, white); font-size: 1.7rem; font-weight: 700; cursor: pointer; }
  .bp-button.secondary { border-color: var(--border); background: var(--surface-2); color: var(--text); }
  .bp-button:disabled, .filter-chip:disabled { opacity: .45; cursor: default; }
  :global([data-bp-focus]:focus-visible) { outline: 3px solid var(--accent); outline-offset: 4px; }
  .search-button { min-width: 24rem; max-width: 48rem; justify-content: flex-start; overflow: hidden; }
  .search-button span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .clear-button { min-width: 5.8rem; }
  .filter-panel { display: flex; flex-direction: column; gap: 1.3rem; padding: 2rem; border: 1px solid var(--border); border-radius: 1.6rem; background: color-mix(in srgb, var(--surface-2) 78%, transparent); }
  .filter-caption, .sort-strip { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; }
  .filter-caption { justify-content: space-between; color: var(--text-2); font-size: 1.5rem; font-weight: 700; }
  .count, .filter-chip span { color: var(--text-3); font-size: 1.3rem; font-weight: 500; }
  .filter-strip, .sort-strip { display: flex; align-items: center; flex-wrap: wrap; gap: .8rem; }
  .filter-chip, .sort-button { min-height: 4.6rem; display: inline-flex; align-items: center; justify-content: center; gap: .8rem; padding: 0 1.5rem; border: 1px solid var(--border); border-radius: 999px; background: var(--surface-1); color: var(--text-2); font-size: 1.45rem; font-weight: 650; cursor: pointer; }
  .filter-chip.selected, .sort-button.selected { border-color: color-mix(in srgb, var(--accent) 75%, white); background: color-mix(in srgb, var(--accent) 18%, var(--surface-1)); color: var(--text); }
  .sort-strip { padding-top: .5rem; }
  .sort-label { margin-right: .5rem; color: var(--text-3); font-size: 1.4rem; }
  .notice { display: flex; align-items: center; justify-content: space-between; gap: 1.5rem; padding: 1.4rem 1.8rem; border: 1px solid var(--border); border-radius: 1.2rem; background: var(--surface-2); color: var(--text-2); font-size: 1.5rem; }
  .notice.error { border-color: color-mix(in srgb, var(--danger) 40%, var(--border)); color: var(--danger); }
  .compact { min-height: 4.8rem; padding: 0 1.4rem; font-size: 1.45rem; }
  .catalog-grid { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 2.4rem 1.6rem; }
  .catalog-card { display: flex; min-width: 0; flex-direction: column; gap: 1rem; padding: .8rem; border: 1px solid transparent; border-radius: 1.5rem; background: transparent; color: var(--text); text-align: left; cursor: pointer; transition: background 120ms ease, border-color 120ms ease, transform 120ms ease; }
  .catalog-card:hover { border-color: var(--border); background: var(--surface-2); transform: translateY(-2px); }
  .cover { display: block; overflow: hidden; width: 100%; border-radius: 1.2rem; background: var(--surface-3); }
  .game-copy { display: flex; min-width: 0; flex-direction: column; gap: .45rem; padding: 0 .4rem .4rem; }
  .game-copy strong, .meta { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
  .game-copy strong { color: var(--text); font-size: 1.65rem; line-height: 1.25; }
  .meta { color: var(--text-3); font-size: 1.3rem; }
  .bp-empty { display: flex; min-height: 28rem; flex-direction: column; align-items: center; justify-content: center; padding: 4rem; border: 1px dashed var(--border); border-radius: 1.6rem; text-align: center; }
  .empty-mark { color: var(--accent); font-size: 5.5rem; }
  .bp-empty h2 { margin: 1rem 0 .5rem; font-size: 2.8rem; }
  .bp-empty p { max-width: 54rem; margin: 0 0 2rem; color: var(--text-2); font-size: 1.7rem; line-height: 1.5; }
  .pagination { display: flex; align-items: center; justify-content: center; gap: 2rem; padding-top: 1rem; }
  .page-button { min-width: 15rem; }
  .page-count { color: var(--text-2); font-size: 1.6rem; font-variant-numeric: tabular-nums; }
  .loading-grid { opacity: .64; }
  .skeleton-card { display: flex; flex-direction: column; gap: 1rem; }
  .skeleton-cover { aspect-ratio: 3 / 4; border-radius: 1.2rem; background: var(--surface-3); animation: pulse 1.4s ease-in-out infinite alternate; }
  .skeleton-line { width: 75%; height: 1.5rem; border-radius: 1rem; background: var(--surface-3); }
  @keyframes pulse { to { opacity: .48; } }
  @media (max-width: 1100px) { .catalog-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); } }
  @media (max-width: 680px) { .bp-catalog { padding-inline: 1.4rem; } .bp-page-header { align-items: stretch; flex-direction: column; } .head-actions { align-items: stretch; flex-direction: column; } .search-button { max-width: none; } .catalog-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1.2rem; } .filter-panel { padding: 1.4rem; } .pagination { gap: .8rem; } .page-button { min-width: 0; flex: 1; padding-inline: 1rem; } }
</style>
