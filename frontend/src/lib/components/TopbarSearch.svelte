<script lang="ts">
  import { ArrowRight, Bell, BellRing } from '@lucide/svelte';
  import { onDestroy } from 'svelte';
  import { SearchOverlay, initialState, rowsOf } from '../search/overlay';
  import { searchAll, type GameHit, type ReleaseHit } from '../services/search';
  import { navigate } from '../stores/router';
  import { toggleWatch, watchedIds } from '../stores/watchlist';
  import { clickOutside } from '../utils/clickOutside';
  import { bytesSize } from '../utils/format';
  import { searchShortcut } from '../utils/shortcut';
  import { msg } from '../i18n';
  import Artwork from './Artwork.svelte';
  import IconButton from './IconButton.svelte';
  import SearchInput from './SearchInput.svelte';

  let search = $state('');
  let searchInput = $state<HTMLInputElement | undefined>(undefined);
  let results = $state(initialState());
  let resultsBox = $state<HTMLElement | undefined>(undefined);

  const overlay = new SearchOverlay({
    search: searchAll,
    onState: (state) => (results = state),
  });

  onDestroy(() => overlay.destroy());

  const total = $derived(results.games.length + results.releases.length);
  const rows = $derived(rowsOf(results));
  const firstRelease = $derived(rows.length - results.releases.length);
  const showEmpty = $derived(
    results.searched && !results.loading && !results.error && total === 0,
  );

  $effect(() => {
    const index = results.active;
    if (index < 0 || !resultsBox) return;
    resultsBox.querySelectorAll('[data-row]')[index]?.scrollIntoView({ block: 'nearest' });
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

  function openActive() {
    const row = overlay.activeRow();
    if (!row) return;
    if (row.kind === 'game') openGame(row.hit);
    else if (row.kind === 'release') openRelease(row.hit);
    else openCatalog();
  }

  function onSearchKeydown(event: KeyboardEvent) {
    switch (event.key) {
      case 'Escape':
        if (results.open) overlay.close();
        else searchInput?.blur();
        return;
      case 'ArrowDown':
        event.preventDefault();
        overlay.move(1);
        return;
      case 'ArrowUp':
        event.preventDefault();
        overlay.move(-1);
        return;
      case 'Enter':
        event.preventDefault();
        if (results.active < 0) openCatalog();
        else openActive();
    }
  }

  function onWindowKeydown(event: KeyboardEvent) {
    if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'k') return;
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
</script>

<svelte:window onkeydown={onWindowKeydown} />

<div class="no-drag search-wrap" data-part="topbar-search" use:clickOutside={() => overlay.close()}>
  <SearchInput
    bind:value={search}
    bind:input={searchInput}
    placeholder={msg('search.placeholder')}
    shortcut={searchShortcut()}
    variant="island"
    loading={results.loading}
    oninput={(value) => overlay.setQuery(value)}
    onfocus={() => overlay.focus()}
    onkeydown={onSearchKeydown}
  />
  {#if results.open && results.query.trim() !== ''}
    <div class="results" bind:this={resultsBox}>
      {#if results.games.length > 0}
        <div class="results-head">{msg('search.games')}</div>
        {#each results.games as hit, i (hit.id)}
          <div class="result-row">
          <button class="result" class:active={results.active === i} data-row onclick={() => openGame(hit)}>
            <span class="result-art">
              <Artwork src={hit.cover ?? ''} alt={hit.title} ratio="3 / 4" radius="var(--radius-xs)" />
            </span>
            <span class="result-body">
              <span class="result-line">
                <span class="result-title">{hit.title}</span>
                {#if hit.installed}
                  <span class="result-badge">{msg('ui.installed')}</span>
                {/if}
              </span>
              {#if gameHint(hit)}
                <span class="result-hint">{gameHint(hit)}</span>
              {/if}
            </span>
          </button>
          {#if !hit.installed}
            {@const watching = $watchedIds.has(hit.id)}
            <IconButton
              size="sm"
              label={watching ? msg('games.unwatchAction') : msg('games.watchAction')}
              lit={watching}
              onclick={() => toggleWatch(hit.id)}
            >
              {#if watching}
                <BellRing size="1.4rem" strokeWidth={1.8} />
              {:else}
                <Bell size="1.4rem" strokeWidth={1.8} />
              {/if}
            </IconButton>
          {/if}
          </div>
        {/each}
        {#if results.moreGames > 0}
          <button class="results-more results-link" class:active={results.active === results.games.length} data-row onclick={openCatalog}>
            {msg('search.moreInCatalog', { count: results.moreGames })}
            <ArrowRight size="1.4rem" strokeWidth={1.8} />
          </button>
        {/if}
      {/if}

      {#if results.releases.length > 0}
        <div class="results-head">{msg('search.releasesNoMatch')}</div>
        {#each results.releases as hit, i (hit.id)}
          <button
            class="result"
            class:active={results.active === firstRelease + i}
            data-row
            onclick={() => openRelease(hit)}
          >
            <span class="result-body">
              <span class="result-title">{hit.title}</span>
              <span class="result-hint">
                {hit.sourceName}{hit.size > 0 ? ` · ${bytesSize(hit.size)}` : ''}
              </span>
            </span>
          </button>
        {/each}
        {#if results.moreReleases > 0}
          <div class="results-more">{msg('search.more', { count: results.moreReleases })}</div>
        {/if}
      {/if}

      {#if results.error}
        <div class="results-error">{results.error}</div>
      {:else if showEmpty}
        <div class="results-empty">{msg('search.nothingFound')}</div>
      {/if}
    </div>
  {/if}
</div>

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
    position: absolute;
    z-index: var(--z-popover);
    top: calc(100% + 0.6rem);
    left: 0;
    right: 0;
    max-height: 46rem;
    overflow-y: auto;
    padding: 0.5rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
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

  .result-row > .result {
    flex: 1;
    min-width: 0;
  }

  .result {
    display: flex;
    align-items: center;
    gap: 1rem;
    width: 100%;
    padding: 0.6rem 1rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
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

  .result:hover,
  .result.active {
    background: var(--hover-strong);
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

  .results-more,
  .results-empty,
  .results-error {
    padding: 0.8rem 1rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .results-error {
    color: var(--danger);
  }

  .results-link {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    border-radius: var(--radius-sm);
    color: var(--accent-text);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .results-link:hover,
  .results-link.active {
    background: var(--hover-strong);
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(-0.3rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
