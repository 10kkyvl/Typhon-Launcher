<script lang="ts">
  import { ArrowRight, Bell, BellRing } from '@lucide/svelte';
  import { onDestroy, untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { layer, topModalLayer } from '../layers';
  import { SearchOverlay, initialState, rowsOf, type Row } from '../search/overlay';
  import { searchAll, type GameHit, type ReleaseHit } from '../services/search';
  import { navigate, route } from '../stores/router';
  import { toggleWatch, watchedIds } from '../stores/watchlist';
  import { bytesSize } from '../utils/format';
  import { anchorTo } from '../utils/position';
  import { searchShortcut } from '../utils/shortcut';
  import { msg } from '../i18n';
  import Artwork from './Artwork.svelte';
  import IconButton from './IconButton.svelte';
  import SearchInput from './SearchInput.svelte';

  /**
   * The top bar's search: an editable combobox whose popup is a grid (WAI-ARIA APG). A result row has
   * two things to act on, the result and its watch toggle, hence a grid and not a listbox. Focus stays
   * in the field the whole time; the cell the keys are on is conveyed with `aria-activedescendant`.
   * The results are a non-modal layer owned by the search island (see lib/layers): the stack answers
   * Escape, presses outside and a dialog opening. What is searched, the debounce and the rows live in
   * lib/search/overlay.ts; this component renders them.
   */
  const uid = $props.id();
  const gridId = `${uid}-results`;
  const headId = (group: 'games' | 'releases') => `${uid}-${group}`;
  // By the result, not its place: the active cell keeps its id through a refresh of the results.
  const cellId = (row: Row, cell: number) => `${uid}-${row.key.replace(/[^\w-]/g, '_')}-${cell}`;

  let search = $state('');
  let searchInput = $state<HTMLInputElement | undefined>(undefined);
  let island = $state<HTMLElement>();
  let results = $state(initialState());

  const overlay = new SearchOverlay({
    search: searchAll,
    onState: (state) => (results = state),
  });

  onDestroy(() => overlay.destroy());

  const rows = $derived(rowsOf(results));
  const firstRelease = $derived(rows.length - results.releases.length);
  const total = $derived(results.games.length + results.releases.length);
  const settled = $derived(results.searched && !results.loading);
  const showEmpty = $derived(settled && !results.error && total === 0);
  // Nothing to show yet (the first search is still running) shows nothing, not an empty box.
  const shown = $derived(results.open && results.query.trim() !== '' && (rows.length > 0 || results.error !== '' || showEmpty));
  const expanded = $derived(shown && rows.length > 0);
  const activeRow = $derived(expanded && results.active >= 0 ? rows[results.active] : undefined);
  const activeCell = $derived(activeRow ? cellId(activeRow, results.cell) : undefined);

  // What a search came to, for a screen reader; that it is running is the field's own to say.
  const outcome = $derived.by(() => {
    if (results.query.trim() === '' || !settled) return '';
    if (results.error) return results.error;
    return total === 0 ? msg('search.nothingFound') : msg('search.found', { count: total });
  });

  // The results belong to the page they were opened on: a route change, whatever made it, closes them.
  const routeKey = $derived($route.key);
  $effect(() => {
    void routeKey;
    untrack(() => overlay.close());
  });

  function openGame(hit: GameHit) {
    overlay.close();
    navigate('game', { id: hit.id });
  }

  function openRelease(hit: ReleaseHit) {
    overlay.close();
    navigate('sources', { sourceId: hit.sourceId, releaseId: hit.id });
  }

  function openCatalog() {
    const query = results.query.trim();
    if (query === '') return;
    overlay.close();
    searchInput?.blur();
    navigate('catalog', { q: query });
  }

  // toggleWatch reports its own failure (a toast); the results stay as they are.
  function watch(hit: GameHit) {
    void toggleWatch(hit.id);
  }

  // Enter: the active cell, or with the keys still in the text the catalog's own search.
  function activate() {
    const row = activeRow;
    if (!row || row.kind === 'more') openCatalog();
    else if (row.kind === 'release') openRelease(row.hit);
    else if (results.cell === 1) watch(row.hit);
    else openGame(row.hit);
  }

  function onSearchKeydown(event: KeyboardEvent) {
    if (event.isComposing) return;
    const { key } = event;
    if (key === 'Enter') {
      event.preventDefault();
      // A held key acts once: its repeats would otherwise search the catalog after opening a result.
      if (!event.repeat) activate();
      return;
    }
    if (event.ctrlKey || event.metaKey || event.altKey) return;
    switch (key) {
      case 'ArrowDown':
      case 'ArrowUp':
        event.preventDefault();
        if (!results.open) overlay.focus();
        overlay.move(key === 'ArrowDown' ? 1 : -1);
        return;
      case 'ArrowRight':
      case 'ArrowLeft':
        // With no row active these are the caret's keys.
        if (expanded && overlay.moveCell(key === 'ArrowRight' ? 1 : -1)) event.preventDefault();
        return;
      case 'Home':
      case 'End':
        // The text's keys: back from the rows to the caret.
        overlay.leave();
        return;
      case 'Escape':
        // Reaches the field only with the results closed (the layer stack answers it while they are
        // open): the second Escape clears the text.
        if (search === '') return;
        search = '';
        overlay.setQuery('');
    }
  }

  function onInput(value: string) {
    // Typing takes the keys back to the text.
    overlay.leave();
    overlay.setQuery(value);
  }

  // A press on the island beside the text (the icon, the padding) is a press on the field: it takes
  // focus, and with the results open it does not take focus away, which would close them.
  function onIslandMousedown(event: MouseEvent) {
    if (event.target === searchInput) return;
    event.preventDefault();
    searchInput?.focus();
  }

  function onShortcut(event: KeyboardEvent) {
    if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'k') return;
    // A dialog owns the keyboard: the page under it is inert and takes no focus from it.
    if (topModalLayer()) return;
    event.preventDefault();
    searchInput?.focus();
    overlay.focus();
  }

  function gameHint(hit: GameHit) {
    const parts: string[] = [];
    if (hit.year) parts.push(String(hit.year));
    const version = hit.latestVersion || hit.version;
    if (version) parts.push(version);
    if (hit.releases > 0) {
      parts.push(msg('search.releases', { count: hit.releases }));
    }
    if (hit.sources > 1) {
      parts.push(msg('search.sources', { count: hit.sources }));
    }
    return parts.join(' · ');
  }

  // Under the island and exactly as wide: anchorTo gives the island's width as min-width, the
  // stylesheet no width of its own.
  const place: Attachment<HTMLElement> = (node) =>
    untrack(() => anchorTo(node, () => island, { placement: 'bottom-start', offset: 6, matchWidth: true, onhide: () => overlay.close() }));

  // Scrolls the results, and only them, to the active row. Runs again when the active cell changes.
  const reveal: Attachment<HTMLElement> = (node) => {
    const id = activeCell;
    untrack(() => {
      const row = id ? document.getElementById(id)?.closest<HTMLElement>('[role="row"]') : null;
      if (!row) return;
      const top = row.offsetTop;
      const bottom = top + row.offsetHeight;
      if (top < node.scrollTop) node.scrollTop = top;
      else if (bottom > node.scrollTop + node.clientHeight) node.scrollTop = bottom - node.clientHeight;
    });
  };
</script>

<svelte:window onkeydown={onShortcut} />

<!-- svelte-ignore a11y_no_static_element_interactions (a pointer convenience: the field itself is the control) -->
<div class="no-drag search-wrap" data-part="topbar-search" bind:this={island} onmousedown={onIslandMousedown}>
  <SearchInput
    bind:value={search}
    bind:input={searchInput}
    placeholder={msg('search.placeholder')}
    shortcut={searchShortcut()}
    variant="island"
    loading={results.loading}
    role="combobox"
    aria-label={msg('search.placeholder')}
    aria-haspopup="grid"
    aria-autocomplete="list"
    aria-expanded={expanded}
    aria-controls={expanded ? gridId : undefined}
    aria-activedescendant={activeCell}
    autocomplete="off"
    oninput={onInput}
    onfocus={() => overlay.focus()}
    onblur={() => overlay.close()}
    onkeydown={onSearchKeydown}
  />
  <span class="sr-only" role="status">{outcome}</span>
</div>

{#snippet cell(row: Row, index: number)}
  <!-- svelte-ignore a11y_click_events_have_key_events (the keys are the field's, see onSearchKeydown) -->
  <div
    class="result"
    class:link={row.kind === 'more'}
    class:active={results.active === index && results.cell === 0}
    role="gridcell"
    id={cellId(row, 0)}
    tabindex="-1"
    onclick={() => (row.kind === 'game' ? openGame(row.hit) : row.kind === 'release' ? openRelease(row.hit) : openCatalog())}
    onpointermove={() => overlay.setActive(index, 0)}
  >
    {#if row.kind === 'game'}
      <span class="result-art">
        <Artwork src={row.hit.cover ?? ''} alt="" ratio="3 / 4" radius="var(--radius-xs)" />
      </span>
      <span class="result-body">
        <span class="result-line">
          <span class="result-title">{row.hit.title}</span>
          {#if row.hit.installed}
            <span class="result-badge">{msg('ui.installed')}</span>
          {/if}
        </span>
        {#if gameHint(row.hit)}
          <span class="result-hint">{gameHint(row.hit)}</span>
        {/if}
      </span>
    {:else if row.kind === 'release'}
      <span class="result-body">
        <span class="result-title">{row.hit.title}</span>
        <span class="result-hint">
          {row.hit.sourceName}{row.hit.size > 0 ? ` · ${bytesSize(row.hit.size)}` : ''}
        </span>
      </span>
    {:else}
      {msg('search.moreInCatalog', { count: results.moreGames })}
      <ArrowRight size="1.4rem" strokeWidth={1.8} aria-hidden="true" />
    {/if}
  </div>
{/snippet}

{#snippet line(row: Row, index: number)}
  <div class="result-row" role="row">
    {@render cell(row, index)}
    {#if row.kind === 'game' && row.cells === 2}
      {@const watching = $watchedIds.has(row.hit.id)}
      <div
        class="result-watch"
        class:active={results.active === index && results.cell === 1}
        role="gridcell"
        id={cellId(row, 1)}
        tabindex="-1"
        onpointermove={() => overlay.setActive(index, 1)}
      >
        <IconButton
          size="sm"
          tabindex={-1}
          label={watching ? msg('games.unwatchAction') : msg('games.watchAction')}
          lit={watching}
          aria-pressed={watching}
          onclick={() => watch(row.hit)}
        >
          {#if watching}
            <BellRing size="1.4rem" strokeWidth={1.8} />
          {:else}
            <Bell size="1.4rem" strokeWidth={1.8} />
          {/if}
        </IconButton>
      </div>
    {/if}
  </div>
{/snippet}

{#if shown}
  <!-- A press in the results keeps focus in the field, where the keys are. -->
  <div
    class="results"
    data-part="popover"
    data-id="search"
    role="presentation"
    onmousedown={(event) => event.preventDefault()}
    {@attach layer({ modal: false, owner: () => island, dismiss: () => overlay.close() })}
    {@attach place}
    {@attach reveal}
  >
    {#if rows.length > 0}
      <div class="grid" role="grid" id={gridId} aria-label={msg('search.results')} aria-busy={results.loading}>
        {#if firstRelease > 0}
          <div class="results-head" id={headId('games')} role="presentation">{msg('search.games')}</div>
          <div role="rowgroup" aria-labelledby={headId('games')}>
            {#each rows.slice(0, firstRelease) as row, i (row.key)}
              {@render line(row, i)}
            {/each}
          </div>
        {/if}
        {#if results.releases.length > 0}
          <div class="results-head" id={headId('releases')} role="presentation">{msg('search.releasesNoMatch')}</div>
          <div role="rowgroup" aria-labelledby={headId('releases')}>
            {#each rows.slice(firstRelease) as row, i (row.key)}
              {@render line(row, firstRelease + i)}
            {/each}
          </div>
        {/if}
      </div>
      {#if results.releases.length > 0 && results.moreReleases > 0}
        <div class="results-note">{msg('search.more', { count: results.moreReleases })}</div>
      {/if}
    {/if}

    {#if results.error}
      <div class="results-note error">{results.error}</div>
    {:else if showEmpty}
      <div class="results-note">{msg('search.nothingFound')}</div>
    {/if}
  </div>
{/if}

<style>
  .no-drag {
    --wails-draggable: no-drag;
  }

  .search-wrap {
    position: relative;
    flex: 1 1 var(--topbar-search-max);
    min-width: var(--topbar-search-min);
    max-width: var(--topbar-search-max);
  }

  .results {
    z-index: var(--z-layer);
    width: 0;
    max-height: 46rem;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 0.5rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: popup-in var(--dur-fast) var(--ease);
  }

  .grid {
    transition: opacity var(--dur) var(--ease);
  }

  /* Results still on screen while a newer search runs. */
  .grid[aria-busy='true'] {
    opacity: 0.6;
  }

  .results-head {
    padding: 0.8rem 1rem 0.5rem;
    font-size: 1.1rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
  }

  .result-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .result {
    display: flex;
    flex: 1;
    align-items: center;
    gap: 1rem;
    min-width: 0;
    padding: 0.6rem 1rem;
    border-radius: var(--radius-sm);
    cursor: default;
    transition: background var(--dur-fast) var(--ease);
  }

  .result.active {
    background: var(--hover-strong);
  }

  .result:focus-visible {
    outline: none;
  }

  .result.link {
    gap: 0.6rem;
    padding: 0.8rem 1rem;
    font-size: var(--font-xs);
    color: var(--accent-text);
  }

  .result-watch {
    display: flex;
    flex: none;
    border-radius: var(--radius-sm);
  }

  /* The keys are on the watch toggle: the ring a focused control would have. */
  .result-watch.active {
    box-shadow: 0 0 0 2px var(--accent-ring);
  }

  .result-art {
    width: 3rem;
    flex-shrink: 0;
  }

  .result-body {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    flex: 1;
  }

  .result-line {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
  }

  .result-badge {
    flex-shrink: 0;
    padding: 1px 0.6rem;
    border-radius: var(--radius-xs);
    background: var(--hover-strong);
    font-size: 1.1rem;
    color: var(--text-3);
  }

  .result-title {
    font-size: var(--font-sm);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .result-hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .results-note {
    padding: 0.8rem 1rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .results-note.error {
    color: var(--danger);
  }
</style>
