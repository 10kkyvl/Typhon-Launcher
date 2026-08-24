<script lang="ts">
  import { ArrowDownUp, ChevronDown, LayoutGrid, List } from '@lucide/svelte';
  import { onDestroy, onMount } from 'svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { playGame, stopGame } from '../../lib/services/library';
  import { queryCatalogGames, type CatalogGame } from '../../lib/services/sources';
  import { installedGames, runningGames } from '../../lib/stores/library';
  import { gameArt, gameInfo, loadArt, requestArt } from '../../lib/stores/metadata';
  import { currentRouteKey, navigate, recallRoute, stashRoute } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { catalogView } from '../../lib/stores/ui';
  import { plural } from '../../lib/utils/format';
  import { inview } from '../../lib/utils/inview';

  type Sort = 'title' | 'year' | 'added';

  const pageSize = 60;
  const sortLabels: Record<Sort, string> = {
    title: 'По алфавиту',
    year: 'По году выхода',
    added: 'По дате добавления',
  };

  interface Snapshot {
    search: string;
    sort: Sort;
    items: CatalogGame[];
    total: number;
    page: number;
    failed: boolean;
  }

  const routeKey = currentRouteKey();
  const restored = recallRoute<Snapshot>(routeKey, 'catalog');

  let search = $state(restored?.search ?? '');
  let sort = $state<Sort>(restored?.sort ?? 'title');
  let items = $state<CatalogGame[]>(restored?.items ?? []);
  let total = $state(restored?.total ?? 0);
  let page = $state(restored?.page ?? 0);
  let loading = $state(!restored);
  let appending = $state(false);
  let failed = $state(restored?.failed ?? false);

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

  async function fetchPage(next: number) {
    const current = ++token;
    loading = true;
    appending = next > 1;
    try {
      const result = await queryCatalogGames({ search, sort, page: next, pageSize });
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

  onMount(() => {
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

  function meta(game: CatalogGame) {
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
      toast(err instanceof Error && err.message ? err.message : 'Не удалось запустить игру', 'danger');
    }
  }

  const subtitle = $derived(
    failed
      ? 'Не удалось загрузить каталог'
      : total === 0
        ? ''
        : `${total} ${plural(total, 'игра', 'игры', 'игр')} из подключённых источников`,
  );
</script>

<PageHeader title="Все игры" {subtitle}>
  {#snippet actions()}
    <SegmentedControl
      bind:value={$catalogView}
      options={[
        { id: 'grid', label: 'Сетка' },
        { id: 'list', label: 'Список' },
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
  {/snippet}
</PageHeader>

<div class="toolbar">
  <div class="search-slot">
    <SearchInput bind:value={search} placeholder="Поиск по каталогу" loading={loading && !appending} oninput={onSearch} />
  </div>
  <DropdownMenu
    items={[
      { id: 'title', label: sortLabels.title },
      { id: 'year', label: sortLabels.year },
      { id: 'added', label: sortLabels.added },
    ]}
    onselect={(id) => onSort(id as Sort)}
  >
    {#snippet trigger({ open, toggle })}
      <button class="chip" class:open onclick={toggle}>
        <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
        {sortLabels[sort]}
        <ChevronDown size="1.4rem" strokeWidth={1.8} />
      </button>
    {/snippet}
  </DropdownMenu>
</div>

{#if items.length === 0}
  {#if loading}
    <p class="muted">Загрузка каталога…</p>
  {:else if failed}
    <EmptyState
      title="Каталог недоступен"
      description="Не удалось получить список игр. Попробуйте обновить источники."
    />
  {:else if search.trim()}
    <EmptyState title="Ничего не найдено" description="Измените запрос или добавьте источник с этой игрой." />
  {:else}
    <EmptyState
      title="Каталог пуст"
      description="Добавьте источник — игры из его фида появятся здесь."
    />
  {/if}
{:else if $catalogView === 'grid'}
  <div class="grid">
    {#each items as game (game.id)}
      {@const shown = $gameInfo[game.id] ?? game}
      <div class="cell" use:inview={() => see(game.id)}>
        <GameCard
          id={game.id}
          title={shown.title}
          cover={$gameArt[game.id]?.cover ?? ''}
          installed={installedByGame.has(game.id)}
          running={$runningGames.has(installedByGame.get(game.id) ?? '')}
          meta={meta(shown)}
          onplay={() => toggleRun(installedByGame.get(game.id) ?? '')}
        />
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
        <span class="list-meta">{meta(shown)}</span>
        <span class="list-meta right">{installedByGame.has(game.id) ? 'Установлена' : ''}</span>
      </button>
    {/each}
  </div>
{/if}

{#if items.length > 0 && items.length < total}
  <div class="more">
    <Button onclick={() => fetchPage(page + 1)} disabled={loading}>
      {appending ? 'Загрузка…' : 'Показать ещё'}
    </Button>
    <span class="muted">Показано {items.length} из {total}</span>
  </div>
{/if}

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-5);
    flex-wrap: wrap;
  }

  .search-slot {
    flex: 1;
    min-width: 24rem;
    max-width: 46rem;
  }

  .chip {
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

  .chip:hover,
  .chip.open {
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
    min-width: 8rem;
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
