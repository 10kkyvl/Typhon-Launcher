<script lang="ts">
  import { ArrowDownUp, ChevronDown, LayoutGrid, List } from '@lucide/svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import Select from '../../lib/components/Select.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { genreLabel } from '../../lib/recommendations/display';
  import { catalogView } from '../../lib/stores/ui';
  import { msg } from '../../lib/i18n';
  import type { CatalogPageData, Sort } from './catalogPageData.svelte';

  // The search, the filters, the sorting and the view switch, docked over the catalog, and the genre
  // chips under them.
  let { data }: { data: CatalogPageData } = $props();

  let stuck = $state(false);

  function watchStuck(dock: HTMLElement) {
    const chipsRow = dock.nextElementSibling;
    if (!chipsRow || typeof IntersectionObserver === 'undefined' || typeof ResizeObserver === 'undefined') return;
    let root = dock.parentElement;
    while (root && !/auto|scroll/.test(getComputedStyle(root).overflowY)) root = root.parentElement;
    let observer: IntersectionObserver | undefined;
    const resize = new ResizeObserver(() => {
      observer?.disconnect();
      observer = new IntersectionObserver(
        ([entry]) => {
          stuck = !entry.isIntersecting && entry.boundingClientRect.top < (entry.rootBounds?.top ?? 0);
        },
        { root, rootMargin: `-${dock.offsetHeight}px 0px 0px 0px` },
      );
      observer.observe(chipsRow);
    });
    resize.observe(dock);
    return {
      destroy() {
        resize.disconnect();
        observer?.disconnect();
      },
    };
  }
</script>

<div class="toolbar-dock" use:watchStuck>
  <div class="toolbar-row" class:stuck>
    <div class="search-wrap">
      <SearchInput bind:value={data.search} label={msg('games.catalogSearchPlaceholder')} placeholder={msg('games.catalogSearchPlaceholder')} loading={data.pages.loading && !data.pages.appending} oninput={data.onSearch} />
    </div>
    <fieldset class="controls" disabled={!data.ready}>
      <Select value={data.platform} width="20rem" label={msg('games.catalogPlatform')} onchange={data.onPlatform}
        options={[{id:'',label:msg('games.catalogAllPlatforms')}, ...data.pages.platforms.map((p) => ({id:p.label,label:p.label}))]} />
      <Select value={data.kind} width="13rem" label={msg('games.catalogKind')} onchange={data.onKind}
        options={[{id:'',label:msg('games.catalogGames')},{id:'all',label:msg('games.catalogAllContent')},{id:'dlc',label:'DLC'},{id:'demo',label:msg('games.catalogDemos')},{id:'soundtrack',label:msg('games.catalogSoundtracks')},{id:'Bundle',label:msg('games.catalogBundles')},{id:'Edition',label:msg('games.catalogEditions')}]} />
      <DropdownMenu
        items={(['auto', 'for-you', 'popular', 'rating', 'year', 'title'] as Sort[]).map((id) => ({
          id,
          label: data.sortLabels[id],
          checked: data.sort === id,
        }))}
        onselect={(id) => data.onSort(id as Sort)}
      >
        {#snippet trigger({ props })}
          <Chip {...props}>
            <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
            {data.sortLabels[(data.effectiveSort || 'popular') as Sort]}
            <ChevronDown size="1.4rem" strokeWidth={1.8} />
          </Chip>
        {/snippet}
      </DropdownMenu>
      <SegmentedControl
        label={msg('games.viewLabel')}
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
    </fieldset>
  </div>
</div>
<fieldset class="chips" disabled={!data.ready}>
  {#each data.chips as label (label)}
    <Chip variant="outline" selected={(label === data.allGenres ? '' : label) === data.genre} onclick={() => data.onGenre(label)}>
      {label === data.allGenres ? label : genreLabel(label)}
    </Chip>
  {/each}
  <span class="chips-sep" aria-hidden="true"></span>
  <Chip variant="outline" selected={data.hideLibrary} onclick={data.onHideLibrary}>{msg('games.catalogHideLibrary')}</Chip>
  <Chip variant="outline" selected={data.hideNotInterested} onclick={data.onHideNotInterested}>{msg('games.catalogHideDismissed')}</Chip>
  {#if data.compatRelevant}
    <Chip
      variant="outline"
      selected={data.compatOnly}
      hint={msg('games.compatFilterHint')}
      onclick={data.onCompatOnly}
    >
      {msg('games.compatFilterLabel')}
    </Chip>
  {/if}
</fieldset>

<style>
  .toolbar-dock {
    position: sticky;
    top: 0;
    z-index: var(--z-sticky);
  }

  .toolbar-row {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
    padding: var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
  }

  /* Stuck over the scrolled catalog, the toolbar is an island like the top bar's. */
  .toolbar-row.stuck {
    background: var(--island-fill);
    border-radius: var(--radius-lg);
  }

  .toolbar-row::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lift);
    opacity: 0;
    transition: opacity var(--dur-panel) var(--ease);
    pointer-events: none;
  }

  .toolbar-row.stuck::after {
    opacity: 1;
  }

  .search-wrap {
    flex: 1;
    min-width: 16rem;
    max-width: 46rem;
  }

  .controls,
  .chips {
    min-width: 0;
    margin: 0;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
  }

  .controls {
    gap: var(--space-2);
    padding: 0;
    border: 0;
  }

  .chips {
    gap: 0.6rem;
    margin-bottom: var(--space-6);
    padding: var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-top: 0;
    border-radius: 0 0 var(--radius-lg) var(--radius-lg);
  }

  .chips-sep {
    width: 1px;
    height: 2rem;
    margin: 0 0.4rem;
    background: var(--border-strong);
  }
</style>
