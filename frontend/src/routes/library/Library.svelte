<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { BellOff, Pencil, Plus } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import { get } from 'svelte/store';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import CollectionModal from '../../lib/components/CollectionModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameCard, { type GameCardAction } from '../../lib/components/GameCard.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import RandomGameModal from '../../lib/components/RandomGameModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SourceBadge from '../../lib/components/SourceBadge.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import { playGame, stopGame, type LibraryCollection } from '../../lib/services/library';
  import { downloads } from '../../lib/stores/downloads';
  import { collections, collectionsLoadError, collectionsState, loadCollections } from '../../lib/stores/collections';
  import { libraryGenres, requestLibraryGenres } from '../../lib/stores/genres';
  import {
    downloadTitles,
    libraryGames,
    libraryLoaded,
    libraryLoadError,
    loadLibrary,
    requestDownloadTitles,
    runningGames,
    stoppedSinceList,
  } from '../../lib/stores/library';
  import { libraryChoices, librarySort } from '../../lib/stores/libraryPrefs';
  import { gameArt, requestArt } from '../../lib/stores/metadata';
  import { gameContextMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { toggleWatch, watchAlertsLoaded, watchlist } from '../../lib/stores/watchlist';
  import { libraryView } from '../../lib/stores/ui';
  import { ambientHover } from '../../lib/utils/ambientHover';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { focusableIn } from '../../lib/layers/focus';
  import { locale, msg } from '../../lib/i18n';
  import { libraryErrorText } from '../../lib/library/errors';
  import { emptyReason } from '../../lib/library/empty';
  import { createEntryCache, entryCover, entryHeroArt, type LibraryEntry } from '../../lib/library/entries';
  import type { LibraryFilter } from '../../lib/library/filters';
  import { effectiveSort, filterEntries, genreOptions, sortEntries, sourceOptions, type LibraryQuery } from '../../lib/library/query';
  import { buildSlides, recentEntries } from '../../lib/library/spotlight';
  import LibraryHero from './LibraryHero.svelte';
  import RecentRow from './RecentRow.svelte';
  import LibraryToolbar from './LibraryToolbar.svelte';
  import { entryMeta, entrySubtitle, narrowingText } from './entryText';

  const saved = get(libraryChoices);
  let filter = $state(saved.filter);
  let sort = $state(get(librarySort));
  let sourceFilter = $state(saved.source);
  let statusFilter = $state(saved.status);
  let search = $state(saved.search);
  let heroHidden = $state(saved.heroHidden);
  let activeCollectionId = $state(saved.collection);
  let collectionOpen = $state(false);
  let randomOpen = $state(false);
  let collectionTarget = $state<LibraryCollection | null>(null);
  let libraryRetrying = $state(false);
  let collectionsRetrying = $state(false);
  let results = $state<HTMLElement>();
  let listSection = $state<HTMLElement>();

  $effect(() => {
    libraryChoices.set({ filter, source: sourceFilter, status: statusFilter, collection: activeCollectionId, search, heroHidden });
  });

  $effect(() => {
    librarySort.set(sort);
  });

  const cache = createEntryCache();
  const entries = $derived(cache.library($libraryGames, $downloads, $downloadTitles));
  const watched = $derived(cache.watched($watchlist));

  // Id lists as strings: a progress tick that changes no id re-runs none of the requests below.
  const downloadIds = $derived(idList(entries.filter((entry) => entry.kind === 'download')));
  const artIds = $derived(idList([...entries, ...watched]));
  const genreIds = $derived(idList(entries.filter((entry) => entry.kind === 'game')));

  function idList(list: readonly LibraryEntry[]): string {
    return list.flatMap((entry) => (entry.catalogId ? [entry.catalogId] : [])).join('\n');
  }

  function ids(list: string): string[] {
    return list ? list.split('\n') : [];
  }

  $effect(() => {
    const wanted = ids(downloadIds);
    untrack(() => void requestDownloadTitles(wanted));
  });

  $effect(() => {
    const wanted = ids(artIds);
    untrack(() => requestArt(wanted));
  });

  $effect(() => {
    const wanted = ids(genreIds);
    untrack(() => void requestLibraryGenres(wanted));
  });

  const activeCollection = $derived($collections.find((c) => c.id === activeCollectionId));

  $effect(() => {
    if (activeCollectionId && $collectionsState === 'ready' && !activeCollection) activeCollectionId = '';
  });

  const knownGenres = $derived(genreOptions(entries, $libraryGenres));

  function openCollection(target: LibraryCollection | null) {
    collectionTarget = target;
    collectionOpen = true;
  }

  const sectionTitles: Record<LibraryFilter, string> = {
    all: msg('games.allGamesTitle'),
    installed: msg('games.filterInstalled'),
    uninstalled: msg('games.notInstalledStatusWord'),
    recent: msg('games.recentLabel'),
    favorites: msg('library.filterFavorites'),
    hidden: msg('games.filterHidden'),
    watched: msg('games.filterWatched'),
  };

  // "Watched" with no watched game returns to "All": when the last one is unwatched here, or when the
  // watchlist is known to be empty (watchAlertsLoaded is set with the first full read of it). A
  // "Watched" restored before that read stays, with its chip on screen.
  let hadWatched = false;
  $effect(() => {
    const count = $watchlist.length;
    const known = $watchAlertsLoaded;
    untrack(() => {
      if (filter === 'watched' && count === 0 && (hadWatched || known)) filter = 'all';
      hadWatched = count > 0;
    });
  });

  function watchedActions(entry: LibraryEntry): GameCardAction[] | undefined {
    if (entry.kind !== 'watched') return undefined;
    return [
      {
        id: 'unwatch',
        label: msg('games.unwatchAction'),
        icon: BellOff,
        onclick: (event) => {
          event.stopPropagation();
          void toggleWatch(entry.id);
        },
      },
    ];
  }

  const librarySources = $derived(sourceOptions(entries));

  $effect(() => {
    if (sourceFilter !== 'all' && $libraryLoaded && !librarySources.includes(sourceFilter)) sourceFilter = 'all';
  });

  const query = $derived<LibraryQuery>({
    filter,
    source: sourceFilter,
    status: statusFilter,
    collection: activeCollection,
    search,
  });
  // Only the play order reads which games just stopped: another sort is not re-run when one stops.
  const stopped = $derived(filter === 'recent' || sort === 'recent' ? $stoppedSinceList : undefined);
  const filtered = $derived(filterEntries(entries, watched, query, $libraryGenres, stopped));
  const visibleGames = $derived(sortEntries(filtered, effectiveSort(filter, sort), $locale, stopped));

  const loadFailed = $derived($libraryLoadError != null && !$libraryLoaded);
  const reason = $derived(
    emptyReason({
      loaded: $libraryLoaded,
      failed: loadFailed,
      entries: entries.length,
      watched: watched.length,
      shown: visibleGames.length,
      query,
    }),
  );

  function clearNarrowing() {
    const handOff = focusHandOff();
    search = '';
    filter = 'all';
    sourceFilter = 'all';
    statusFilter = 'all';
    activeCollectionId = '';
    // The list's first game when it fills; the search field when it stays empty (all games hidden).
    void handOff(() => (results ? focusableIn(results)[0] : undefined) ?? (listSection ? focusableIn(listSection)[0] : undefined));
  }

  const hero = $derived(buildSlides(entries, { running: $runningGames, stopped: $stoppedSinceList, locale: $locale })[0]);
  const recentGames = $derived(
    recentEntries(entries, { shown: hero && !heroHidden ? hero.id : undefined, stopped: $stoppedSinceList, locale: $locale }),
  );

  let lastView = get(libraryView);
  let viewSwitched = $state(false);

  $effect(() => {
    if ($libraryView === lastView) return;
    lastView = $libraryView;
    viewSwitched = true;
  });

  function endViewSwitch(event: AnimationEvent) {
    if (event.target === event.currentTarget) viewSwitched = false;
  }

  async function toggleRun(id: string) {
    try {
      if ($runningGames.has(id)) await stopGame(id);
      else await playGame(id);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
    }
  }
</script>

<Page>
  <PageHeader title={msg('games.libraryWord')} />

  {#if hero && !heroHidden}
    <LibraryHero
      {hero}
      art={entryHeroArt(hero, $gameArt)}
      running={$runningGames.has(hero.id)}
      onplay={() => toggleRun(hero.id)}
      onhide={() => (heroHidden = true)}
    />
  {/if}

  {#if recentGames.length > 0}
    <RecentRow
      entries={recentGames}
      cover={(entry) => entryHeroArt(entry, $gameArt) || entryCover(entry, $gameArt)}
      running={$runningGames}
      onshowall={() => (filter = 'recent')}
      onplay={toggleRun}
    />
  {/if}

  <section class="section" bind:this={listSection}>
    <div class="section-head">
      <h2>{activeCollection ? activeCollection.name : sectionTitles[filter]}</h2>
    </div>

    <LibraryToolbar
      bind:search
      bind:filter
      bind:sort
      bind:sourceFilter
      bind:statusFilter
      sources={librarySources}
      watched={$watchlist.length > 0 || filter === 'watched'}
      onrandom={() => (randomOpen = true)}
    />

    {#if $collectionsState === 'failed' || collectionsRetrying}
      <div class="collections-failed">
        <Notice
          tone="danger"
          compact
          retry={{ label: msg('common.retry'), run: () => loadCollections() }}
          bind:retrying={collectionsRetrying}
        >
          {libraryErrorText($collectionsLoadError, msg('games.collectionsLoadFailed'))}
        </Notice>
      </div>
    {/if}

    <div class="collections" role="group" aria-label={msg('games.collectionsLabel')}>
      {#each $collections as collection (collection.id)}
        <Chip
          selected={activeCollectionId === collection.id}
          onclick={() => (activeCollectionId = activeCollectionId === collection.id ? '' : collection.id)}
        >
          {collection.name}
        </Chip>
      {/each}
      {#if activeCollection}
        <IconButton size="sm" label={msg('games.collectionEditLabel')} onclick={() => openCollection(activeCollection)}>
          <Pencil size="1.4rem" strokeWidth={1.8} />
        </IconButton>
      {/if}
      <Chip variant="quiet" onclick={() => openCollection(null)}>
        <Plus size="1.4rem" strokeWidth={1.8} />
        {msg('games.collectionAdd')}
      </Chip>
    </div>

    {#if loadFailed || libraryRetrying}
      <div class="load-failed">
        <Notice
          tone="danger"
          title={msg('library.loadFailedTitle')}
          retry={{ label: msg('common.retry'), run: () => loadLibrary() }}
          bind:retrying={libraryRetrying}
        >
          {libraryErrorText($libraryLoadError, msg('library.loadFailedText'))}
        </Notice>
      </div>
    {/if}

    {#if visibleGames.length === 0}
      {#if reason?.kind === 'loading'}
        <EmptyState level={3} title={msg('library.loadingTitle')} />
      {:else if reason?.kind === 'empty'}
        <EmptyState level={3} title={msg('games.libraryEmptyTitle')} description={msg('games.libraryEmptyDescription')} />
      {:else if reason?.kind === 'all-hidden'}
        <EmptyState level={3} title={msg('library.allHiddenTitle')} description={msg('library.allHiddenDescription')} />
      {:else if reason?.kind === 'no-match'}
        <EmptyState level={3} title={msg('games.nothingFoundTitle')} description={narrowingText(reason.narrowing)}>
          {#snippet actions()}
            <Button onclick={clearNarrowing}>{msg('library.clearFilters')}</Button>
          {/snippet}
        </EmptyState>
      {/if}
    {:else if $libraryView === 'grid'}
      <div class="grid" class:switched={viewSwitched} onanimationend={endViewSwitch} bind:this={results}>
        {#each visibleGames as entry (entry.id)}
          <GameCard
            id={entry.id}
            title={entry.title}
            cover={entryCover(entry, $gameArt)}
            ambient
            installed={entry.installed}
            running={$runningGames.has(entry.id)}
            source={entry.source}
            platform={entry.platform}
            meta={entry.kind === 'watched' ? entrySubtitle(entry) : entryMeta(entry)}
            actions={watchedActions(entry)}
            onplay={entry.playable ? () => toggleRun(entry.id) : undefined}
          />
        {/each}
      </div>
    {:else}
      <div class="list" class:switched={viewSwitched} onanimationend={endViewSwitch} bind:this={results}>
        {#each visibleGames as entry (entry.id)}
          <button
            class="list-row"
            use:ambientHover={entry.id}
            onclick={() => navigate('game', { id: entry.id })}
            {@attach gameContextMenu(entry.id)}
          >
            <div class="list-thumb" aria-hidden="true">
              <Artwork src={entryCover(entry, $gameArt)} alt="" label={entry.title} radius="var(--radius-xs)" />
            </div>
            <span class="list-title">{entry.title}</span>
            {#if entry.platform}
              <ConsoleTag platform={entry.platform} />
            {:else if entry.source}
              <SourceBadge source={entry.source} />
            {/if}
            <span class="list-meta">{entrySubtitle(entry)}</span>
            <span class="list-meta right">{entry.kind === 'watched' ? '' : entryMeta(entry)}</span>
          </button>
        {/each}
      </div>
    {/if}
  </section>

  {#if randomOpen}
    <RandomGameModal onclose={() => (randomOpen = false)} />
  {/if}

  {#if collectionOpen}
    <CollectionModal bind:open={collectionOpen} collection={collectionTarget} genres={knownGenres} />
  {/if}
</Page>

<style>
  .section {
    margin-bottom: var(--space-8);
  }

  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .section-head h2 {
    font-size: var(--font-xl);
  }

  .collections-failed,
  .load-failed {
    margin-bottom: var(--space-5);
  }

  .collections {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.6rem;
    margin-bottom: var(--space-5);
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

  .grid.switched,
  .list.switched {
    animation: rise-in var(--dur-panel) var(--ease);
  }

  .list-row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 0.8rem 0.8rem;
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
    min-width: 6rem;
    text-align: right;
    font-variant-numeric: tabular-nums;
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
