<script lang="ts">
  import { ArrowDownUp, Bell, ChevronDown, Clock, Dices, EyeOff, LayoutGrid, List, MonitorDown, Star } from '@lucide/svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { statusLabel } from '../../lib/game/status';
  import { msg } from '../../lib/i18n';
  import { nextLibraryFilter, type LibraryFilter } from '../../lib/library/filters';
  import { sourceLabel, type GameSource } from '../../lib/library/source';
  import { STATUS_FILTERS, type StatusFilter } from '../../lib/library/statuses';
  import { libraryView } from '../../lib/stores/ui';
  import type { LibrarySort } from '../../lib/library/query';

  let {
    search = $bindable(),
    filter = $bindable(),
    sort = $bindable(),
    sourceFilter = $bindable(),
    statusFilter = $bindable(),
    sources: librarySources,
    watched,
    onrandom,
  }: {
    search: string;
    filter: LibraryFilter;
    sort: LibrarySort;
    sourceFilter: GameSource | 'all';
    statusFilter: StatusFilter;
    sources: readonly GameSource[];
    watched: boolean;
    onrandom: () => void;
  } = $props();

  const filters = $derived<{ id: LibraryFilter; label: string; icon?: typeof Clock }[]>([
    { id: 'all', label: msg('games.filterAll') },
    { id: 'installed', label: msg('games.filterInstalled'), icon: MonitorDown },
    { id: 'recent', label: msg('games.recentLabel'), icon: Clock },
    { id: 'favorites', label: msg('library.filterFavorites'), icon: Star },
    { id: 'hidden', label: msg('games.filterHidden'), icon: EyeOff },
    ...(watched ? [{ id: 'watched' as const, label: msg('games.filterWatched'), icon: Bell }] : []),
  ]);

  const sorts: readonly LibrarySort[] = ['title', 'recent', 'playtime', 'size', 'status', 'completed'];

  const sortLabels: Record<LibrarySort, string> = {
    title: msg('games.sortAlpha'),
    recent: msg('games.librarySortRecent'),
    playtime: msg('games.librarySortPlaytime'),
    size: msg('games.sortSize'),
    status: msg('games.sortStatus'),
    completed: msg('games.sortCompleted'),
    added: msg('games.catalogSortAdded'),
  };

  function statusFilterLabel(id: StatusFilter): string {
    if (id === 'all') return msg('games.filterStatusAll');
    if (id === 'none') return msg('games.filterStatusNone');
    return statusLabel(id);
  }
</script>

<div class="toolbar">
  <div class="search-wrap">
    <SearchInput bind:value={search} label={msg('games.librarySearchPlaceholder')} placeholder={msg('games.librarySearchPlaceholder')} />
  </div>
  <div class="toolbar-right">
    <Chip onclick={onrandom}>
      <Dices size="1.4rem" strokeWidth={1.8} />
      {msg('games.randomButton')}
    </Chip>
    {#each filters as f (f.id)}
      <Chip selected={filter === f.id} onclick={() => (filter = nextLibraryFilter(filter, f.id))}>
        {#if f.icon}
          <f.icon size="1.4rem" strokeWidth={1.8} />
        {/if}
        {f.label}
      </Chip>
    {/each}
    {#if librarySources.length >= 2 && filter !== 'watched'}
      <DropdownMenu
        items={[
          { id: 'all', label: msg('games.filterSourceAll'), checked: sourceFilter === 'all' },
          ...librarySources.map((source) => ({
            id: source,
            label: sourceLabel(source),
            checked: sourceFilter === source,
          })),
        ]}
        onselect={(id) => (sourceFilter = id as GameSource | 'all')}
      >
        {#snippet trigger({ props })}
          <Chip {...props} selected={sourceFilter !== 'all'}>
            {sourceFilter === 'all' ? msg('games.filterSourceAll') : sourceLabel(sourceFilter)}
            <ChevronDown size="1.4rem" strokeWidth={1.8} />
          </Chip>
        {/snippet}
      </DropdownMenu>
    {/if}
    {#if filter !== 'watched'}
      <DropdownMenu
        items={STATUS_FILTERS.map((id) => ({ id, label: statusFilterLabel(id), checked: statusFilter === id }))}
        onselect={(id) => (statusFilter = id as StatusFilter)}
      >
        {#snippet trigger({ props })}
          <Chip {...props} selected={statusFilter !== 'all'}>
            {statusFilterLabel(statusFilter)}
            <ChevronDown size="1.4rem" strokeWidth={1.8} />
          </Chip>
        {/snippet}
      </DropdownMenu>
    {/if}
    <DropdownMenu
      items={sorts.map((id) => ({
        id,
        label: sortLabels[id],
        checked: sort === id,
      }))}
      onselect={(id) => (sort = id as LibrarySort)}
    >
      {#snippet trigger({ props })}
        <Chip {...props}>
          <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
          {sortLabels[sort]}
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </Chip>
      {/snippet}
    </DropdownMenu>
    <SegmentedControl
      label={msg('games.viewLabel')}
      bind:value={$libraryView}
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

<style>
  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3) var(--space-4);
    margin-bottom: var(--space-5);
    flex-wrap: wrap;
  }

  .search-wrap {
    flex: 1;
    min-width: 22rem;
    max-width: 38rem;
  }

  .toolbar-right {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }
</style>
