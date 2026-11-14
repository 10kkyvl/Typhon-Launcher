<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { BellOff, Pencil, Plus } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { get } from 'svelte/store';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import CollectionModal from '../../lib/components/CollectionModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import type { GameCardAction } from '../../lib/components/GameCard.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import RandomGameModal from '../../lib/components/RandomGameModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { launchesInFlight, playGame, stopGame, type LibraryCollection } from '../../lib/services/library';
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
  import { toast } from '../../lib/stores/toasts';
  import { toggleWatch, watchAlertsLoaded, watchlist } from '../../lib/stores/watchlist';
  import { libraryView } from '../../lib/stores/ui';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { focusableIn, focusQuietly } from '../../lib/layers/focus';
  import { locale, msg } from '../../lib/i18n';
  import { libraryErrorText } from '../../lib/library/errors';
  import { emptyReason } from '../../lib/library/empty';
  import { createEntryCache, entryCover, entryHeroArt, type LibraryEntry } from '../../lib/library/entries';
  import type { LibraryFilter } from '../../lib/library/filters';
  import { effectiveSort, filterEntries, genreOptions, sortEntries, sourceOptions, type LibraryQuery } from '../../lib/library/query';
  import { buildSlides, RECENT_LIMIT, recentEntries } from '../../lib/library/spotlight';
  import LibraryCarousel from './LibraryCarousel.svelte';
  import RecentRow from './RecentRow.svelte';
  import LibraryToolbar from './LibraryToolbar.svelte';
  import LibraryGrid from './LibraryGrid.svelte';
  import LibraryList from './LibraryList.svelte';
  import { narrowingText } from './entryText';
  import { itemOf } from './itemMenu';
  import { primaryOf } from './roving';

  const uid = $props.id();
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
  let collectionsRow = $state<HTMLElement>();

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

  const unwatching = new SvelteSet<string>();

  // toggleWatch says its own failure; the press waits for it so a second one sends nothing.
  async function unwatch(id: string) {
    if (unwatching.has(id)) return;
    unwatching.add(id);
    try {
      await toggleWatch(id);
    } finally {
      unwatching.delete(id);
    }
  }

  function watchedActions(entry: LibraryEntry): GameCardAction[] | undefined {
    if (entry.kind !== 'watched') return undefined;
    return [
      {
        id: 'unwatch',
        label: msg('games.unwatchAction'),
        icon: BellOff,
        busy: unwatching.has(entry.id),
        onclick: (event) => {
          event.stopPropagation();
          void unwatch(entry.id);
        },
      },
    ];
  }

  // A selected source keeps its menu on screen (it stays among the options), so it is never a narrowing
  // the player cannot see or take back.
  const librarySources = $derived(sourceOptions(entries, { hidden: filter === 'hidden', selected: sourceFilter }));

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

  const slides = $derived(buildSlides(entries, { running: $runningGames, stopped: $stoppedSinceList, locale: $locale }));
  const carouselShown = $derived(!heroHidden && slides.length > 0);
  // The row leaves out what the carousel shows, so nothing is drawn twice at the top.
  const recentGames = $derived.by(() => {
    const onScreen = new Set(carouselShown ? slides.map((entry) => entry.id) : []);
    return recentEntries(entries, { stopped: $stoppedSinceList, locale: $locale, limit: RECENT_LIMIT + onScreen.size })
      .filter((entry) => !onScreen.has(entry.id))
      .slice(0, RECENT_LIMIT);
  });

  let carouselBlock = $state<ReturnType<typeof LibraryCarousel>>();
  let showCarouselSpot = $state<HTMLElement>();

  // The control pressed goes with its block; focus moves to the one that takes it back.
  function setCarouselHidden(hidden: boolean) {
    const handOff = focusHandOff();
    heroHidden = hidden;
    void handOff(() => (hidden ? (showCarouselSpot ? focusableIn(showCarouselSpot)[0] : undefined) : carouselBlock?.hideControl()));
  }

  // The game the list's Tab stop is on, kept across the grid and the list.
  let currentGame = $state<string>();

  // A game that leaves the list (hidden, unwatched, filtered out) does not take focus or the Tab stop
  // with it: both move to the game now in its place, the one before it at the end, and focus to the
  // section's first control when the list empties. Read before the list changes on screen, acted on
  // right after, before anything else picks a place for focus.
  let shownIds: string[] = [];
  let leaving: { focus: number; current: number } | null = null;

  const resultsList = () => listSection?.querySelector<HTMLElement>('[data-results]') ?? null;

  $effect.pre(() => {
    void visibleGames;
    untrack(() => {
      const list = resultsList();
      const active = document.activeElement;
      const item = list && active && list.contains(active) ? itemOf(list, active) : null;
      leaving = {
        focus: list && item ? [...list.children].indexOf(item) : -1,
        current: currentGame === undefined ? -1 : shownIds.indexOf(currentGame),
      };
    });
  });

  $effect(() => {
    const ids = visibleGames.map((entry) => entry.id);
    untrack(() => {
      const was = leaving;
      leaving = null;
      shownIds = ids;
      if (!was) return;
      if (currentGame !== undefined && !ids.includes(currentGame) && was.current >= 0) {
        currentGame = ids[Math.min(was.current, ids.length - 1)];
      }
      if (was.focus < 0) return;
      const active = document.activeElement;
      if (active && active !== document.body && active.isConnected) return;
      const list = resultsList();
      const item = list?.children[Math.min(was.focus, list.children.length - 1)];
      const target = item instanceof HTMLElement ? primaryOf(item) : listSection ? focusableIn(listSection)[0] : undefined;
      if (target) focusQuietly(target);
    });
  });

  let lastView = get(libraryView);
  let viewSwitched = $state(false);

  $effect(() => {
    if ($libraryView === lastView) return;
    lastView = $libraryView;
    viewSwitched = true;
  });

  // A launch or a stop waits on Go: its control is busy and a second press sends nothing. A launch
  // another screen started counts too (the service's set).
  const pressed = new SvelteSet<string>();
  const busy = (id: string) => pressed.has(id) || $launchesInFlight.has(id);

  async function toggleRun(id: string) {
    if (busy(id)) return;
    const stopping = $runningGames.has(id);
    pressed.add(id);
    try {
      if (stopping) await stopGame(id);
      else await playGame(id);
    } catch (err) {
      toast(libraryErrorText(err, msg(stopping ? 'games.errorStopFailed' : 'games.errorPlayFailed')), 'danger');
    } finally {
      pressed.delete(id);
    }
  }
</script>

<Page>
  {#snippet bleed()}
    {#if carouselShown}
      <LibraryCarousel
        bind:this={carouselBlock}
        entries={slides}
        art={$gameArt}
        running={$runningGames}
        {busy}
        onplay={toggleRun}
        onstop={toggleRun}
        onhide={() => setCarouselHidden(true)}
      />
    {/if}
  {/snippet}

  <PageHeader title={msg('games.libraryWord')}>
    {#snippet actions()}
      {#if heroHidden && slides.length > 0}
        <span bind:this={showCarouselSpot}>
          <Button variant="ghost" size="sm" onclick={() => setCarouselHidden(false)}>{msg('library.carousel.show')}</Button>
        </span>
      {/if}
    {/snippet}
  </PageHeader>

  {#if recentGames.length > 0}
    <RecentRow
      entries={recentGames}
      cover={(entry) => entryHeroArt(entry, $gameArt) || entryCover(entry, $gameArt)}
      running={$runningGames}
      {busy}
      onshowall={() => (filter = 'recent')}
      fallback={() => (listSection ? focusableIn(listSection)[0] : undefined)}
      onplay={toggleRun}
    />
  {/if}

  <section class="section" bind:this={listSection}>
    <div class="section-head">
      <h2 id="{uid}-heading">{activeCollection ? activeCollection.name : sectionTitles[filter]}</h2>
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

    <div class="collections" role="group" aria-label={msg('games.collectionsLabel')} bind:this={collectionsRow}>
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
      <LibraryGrid
        entries={visibleGames}
        art={$gameArt}
        running={$runningGames}
        {busy}
        actions={watchedActions}
        onplay={toggleRun}
        labelledby="{uid}-heading"
        bind:current={currentGame}
        bind:switched={viewSwitched}
        bind:element={results}
      />
    {:else}
      <LibraryList
        entries={visibleGames}
        art={$gameArt}
        running={$runningGames}
        {busy}
        onplay={toggleRun}
        labelledby="{uid}-heading"
        bind:current={currentGame}
        bind:switched={viewSwitched}
        bind:element={results}
      />
    {/if}
  </section>

  {#if randomOpen}
    <RandomGameModal onclose={() => (randomOpen = false)} />
  {/if}

  {#if collectionOpen}
    <CollectionModal
      bind:open={collectionOpen}
      collection={collectionTarget}
      genres={knownGenres}
      returnFocus={() => (collectionsRow ? focusableIn(collectionsRow)[0] : undefined)}
    />
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
</style>
