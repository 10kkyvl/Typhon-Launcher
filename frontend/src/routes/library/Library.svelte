<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { BellOff, Pencil, Plus } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import { get } from 'svelte/store';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import CollectionModal from '../../lib/components/CollectionModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameCard, { type GameCardAction } from '../../lib/components/GameCard.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import RandomGameModal from '../../lib/components/RandomGameModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { playGame, stopGame, type LibraryCollection, type LibraryGame } from '../../lib/services/library';
  import { getCatalogGames, type CatalogGame } from '../../lib/services/sources';
  import type { Download } from '../../lib/services/downloads';
  import { downloads, statusLabels } from '../../lib/stores/downloads';
  import { collections } from '../../lib/stores/collections';
  import { libraryGenres, requestLibraryGenres } from '../../lib/stores/genres';
  import { installedGames, libraryGames, runningGames } from '../../lib/stores/library';
  import { gameArt, requestArt } from '../../lib/stores/metadata';
  import { gameContextMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { toggleWatch, watchlist } from '../../lib/stores/watchlist';
  import { libraryView } from '../../lib/stores/ui';
  import { bytesSize } from '../../lib/utils/format';
  import { ambientHover } from '../../lib/utils/ambientHover';
  import { msg } from '../../lib/i18n';
  import { libraryErrorText } from '../../lib/library/errors';
  import { metadataErrorText } from '../../lib/metadata/metadataErrors';
  import type { LibraryFilter } from '../../lib/library/filters';
  import { gameSource, sourceOrder, type GameSource } from '../../lib/library/source';
  import SourceBadge from '../../lib/components/SourceBadge.svelte';
  import { canPlay } from '../../lib/library/launch';
  import { emulationPlatform } from '../../lib/library/emulation';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import { matchesCollection } from '../../lib/library/collections';
  import { compareByCompleted, compareByStatus, matchesStatus, type StatusFilter } from '../../lib/library/statuses';
  import LibraryHero from './LibraryHero.svelte';
  import RecentRow from './RecentRow.svelte';
  import LibraryToolbar from './LibraryToolbar.svelte';
  import type { Entry, Sort } from './entry';

  let filter = $state<LibraryFilter>('all');
  let sort = $state<Sort>('alpha');
  let sourceFilter = $state<GameSource | 'all'>('all');
  let statusFilter = $state<StatusFilter>('all');
  let search = $state('');
  let heroHidden = $state(false);
  let activeCollectionId = $state('');
  let collectionOpen = $state(false);
  let randomOpen = $state(false);
  let collectionTarget = $state<LibraryCollection | null>(null);
  let catalogGames = $state<Record<string, CatalogGame>>({});
  const installedByGame = $derived.by(() => {
    const ids = new Set<string>();
    for (const game of $libraryGames) {
      if (game.canonicalGameId) ids.add(game.canonicalGameId);
    }
    return ids;
  });

  const downloaded = $derived.by(() => {
    const map = new Map<string, Download>();
    for (const item of $downloads) {
      const gameId = item.origin?.gameId;
      if (!gameId || installedByGame.has(gameId)) continue;
      const current = map.get(gameId);
      if (!current || Date.parse(item.addedAt) >= Date.parse(current.addedAt)) map.set(gameId, item);
    }
    return map;
  });

  async function loadCatalogGames(ids: string[]) {
    try {
      const games = await getCatalogGames(ids);
      if (games.length === 0) return;
      catalogGames = { ...catalogGames, ...Object.fromEntries(games.map((game) => [game.id, game])) };
    } catch (err) {
      toast(metadataErrorText(err, msg('games.libraryLoadGamesError')), 'danger');
    }
  }

  $effect(() => {
    const ids = [...downloaded.keys()];
    untrack(() => {
      const missing = ids.filter((id) => !(id in catalogGames));
      if (missing.length > 0) loadCatalogGames(missing);
    });
  });

  $effect(() => {
    requestArt([
      ...downloaded.keys(),
      ...$watchlist.map((item) => item.gameId),
      ...$libraryGames.map((game) => game.canonicalGameId).filter((cid): cid is string => Boolean(cid)),
    ]);
  });

  $effect(() => {
    const ids = $libraryGames.map((game) => game.canonicalGameId).filter((cid): cid is string => Boolean(cid));
    untrack(() => void requestLibraryGenres(ids));
  });

  const activeCollection = $derived($collections.find((c) => c.id === activeCollectionId));

  const knownGenres = $derived([
    ...new Set(
      $libraryGames.flatMap((game) => (game.canonicalGameId ? ($libraryGenres[game.canonicalGameId] ?? []) : [])),
    ),
  ]);

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

  function installedEntry(game: LibraryGame): Entry {
    const bits: string[] = [];
    if (game.uninstalled) {
      bits.push(msg('games.gameNotInstalledWord'));
    } else {
      if (game.version) bits.push(game.version);
      if (game.sizeBytes > 0) bits.push(bytesSize(game.sizeBytes));
    }
    const art = game.canonicalGameId ? $gameArt[game.canonicalGameId] : undefined;
    return {
      id: game.id,
      title: game.title,
      cover: art?.cover || game.cover,
      hero: art?.hero ?? '',
      installed: !game.uninstalled,
      playable: canPlay(game),
      playtimeSeconds: game.playtimeSeconds,
      sizeBytes: game.sizeBytes,
      lastPlayed: game.lastPlayed,
      subtitle: bits.join(' · '),
      hidden: Boolean(game.hidden),
      inLibrary: true,
      tags: game.tags,
      status: game.status,
      completedAt: game.completedAt,
      canonicalGameId: game.canonicalGameId,
      source: gameSource(game),
      platform: emulationPlatform(game),
    };
  }

  function downloadedEntry(gameId: string, item: Download): Entry {
    const game = catalogGames[gameId];
    const bits: string[] = [];
    if (game?.releaseYear) bits.push(String(game.releaseYear));
    if (game?.developer) bits.push(game.developer);
    bits.push(statusLabels(item.status));
    return {
      id: gameId,
      title: game?.title || item.name,
      cover: $gameArt[gameId]?.cover ?? '',
      hero: $gameArt[gameId]?.hero ?? '',
      installed: false,
      playable: false,
      playtimeSeconds: 0,
      sizeBytes: item.total,
      lastPlayed: null,
      subtitle: bits.join(' · '),
      hidden: false,
      inLibrary: false,
      source: 'typhon',
    };
  }

  const watchedEntries = $derived(
    $watchlist.map((item): Entry => ({
      id: item.gameId,
      title: item.title,
      cover: $gameArt[item.gameId]?.cover ?? '',
      hero: $gameArt[item.gameId]?.hero ?? '',
      installed: false,
      playable: false,
      playtimeSeconds: 0,
      sizeBytes: 0,
      lastPlayed: null,
      subtitle: msg('games.watchedMeta'),
      hidden: false,
      inLibrary: false,
      watched: true,
    })),
  );

  $effect(() => {
    if (filter === 'watched' && $watchlist.length === 0) filter = 'all';
  });

  function watchedActions(entry: Entry): GameCardAction[] | undefined {
    if (!entry.watched) return undefined;
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

  const entries = $derived.by(() => {
    const installed = $libraryGames.map(installedEntry);
    const rest = [...downloaded].map(([gameId, item]) => downloadedEntry(gameId, item));
    return [...installed, ...rest];
  });

  const librarySources = $derived(
    sourceOrder.filter((source) => entries.some((entry) => entry.source === source)),
  );

  $effect(() => {
    if (sourceFilter !== 'all' && !librarySources.includes(sourceFilter)) sourceFilter = 'all';
  });

  const visibleGames = $derived.by(() => {
    const query = search.trim().toLowerCase();
    const filtered = (filter === 'watched' ? watchedEntries : entries).filter((entry) => {
      if (query && !entry.title.toLowerCase().includes(query)) return false;
      if (entry.hidden !== (filter === 'hidden')) return false;
      if (filter !== 'watched' && sourceFilter !== 'all' && entry.source !== sourceFilter) return false;
      if (filter !== 'watched' && statusFilter !== 'all' && !(entry.inLibrary && matchesStatus(entry.status, statusFilter))) return false;
      if (activeCollection) {
        const genres = entry.canonicalGameId ? $libraryGenres[entry.canonicalGameId] : undefined;
        if (!entry.inLibrary || !matchesCollection(entry, genres, activeCollection)) return false;
      }
      switch (filter) {
        case 'installed':
          return entry.installed;
        case 'recent':
          return entry.lastPlayed !== null;
        default:
          return true;
      }
    });

    return filtered.toSorted((a, b) => {
      switch (sort) {
        case 'recent':
          return time(b.lastPlayed) - time(a.lastPlayed) || a.title.localeCompare(b.title, 'ru');
        case 'playtime':
          return b.playtimeSeconds - a.playtimeSeconds || a.title.localeCompare(b.title, 'ru');
        case 'size':
          return b.sizeBytes - a.sizeBytes || a.title.localeCompare(b.title, 'ru');
        case 'status':
          return compareByStatus(a, b);
        case 'completed':
          return compareByCompleted(a, b);
        default:
          return a.title.localeCompare(b.title, 'ru');
      }
    });
  });

  function time(value: string | null) {
    if (!value) return 0;
    const parsed = Date.parse(value);
    return Number.isNaN(parsed) ? 0 : parsed;
  }

  const playedGames = $derived(
    $installedGames.filter((game) => !game.hidden).map(installedEntry).toSorted((a, b) => time(b.lastPlayed) - time(a.lastPlayed)),
  );

  const hero = $derived(playedGames.find((entry) => entry.lastPlayed !== null && entry.playable));

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

  const recentGames = $derived(
    playedGames.filter((entry) => entry.lastPlayed !== null && entry.id !== hero?.id),
  );

  function entryMeta(entry: Entry) {
    if (entry.sizeBytes > 0) return bytesSize(entry.sizeBytes);
    return entry.installed ? '' : msg('games.gameNotInstalledWord');
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
      running={$runningGames.has(hero.id)}
      onplay={() => toggleRun(hero.id)}
      onhide={() => (heroHidden = true)}
    />
  {/if}

  {#if recentGames.length > 0}
    <RecentRow entries={recentGames} running={$runningGames} onshowall={() => (filter = 'recent')} onplay={toggleRun} />
  {/if}

  <section class="section">
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
      watched={$watchlist.length > 0}
      onrandom={() => (randomOpen = true)}
    />

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

    {#if visibleGames.length === 0}
      {#if search.trim()}
        <EmptyState level={3} title={msg('games.nothingFoundTitle')} description={msg('games.libraryNothingFoundDescription')} />
      {:else}
        <EmptyState level={3}
          title={msg('games.libraryEmptyTitle')}
          description={msg('games.libraryEmptyDescription')}
        />
      {/if}
    {:else if $libraryView === 'grid'}
      <div class="grid" class:switched={viewSwitched} onanimationend={endViewSwitch}>
        {#each visibleGames as entry (entry.id)}
          <GameCard
            id={entry.id}
            title={entry.title}
            cover={entry.cover}
            ambient
            installed={entry.installed}
            running={$runningGames.has(entry.id)}
            source={entry.source}
            platform={entry.platform}
            meta={entry.watched ? entry.subtitle : entryMeta(entry)}
            actions={watchedActions(entry)}
            onplay={entry.playable ? () => toggleRun(entry.id) : undefined}
          />
        {/each}
      </div>
    {:else}
      <div class="list" class:switched={viewSwitched} onanimationend={endViewSwitch}>
        {#each visibleGames as entry (entry.id)}
          <button
            class="list-row"
            use:ambientHover={entry.id}
            onclick={() => navigate('game', { id: entry.id })}
            {@attach gameContextMenu(entry.id)}
          >
            <div class="list-thumb" aria-hidden="true">
              <Artwork src={entry.cover} alt="" label={entry.title} radius="var(--radius-xs)" />
            </div>
            <span class="list-title">{entry.title}</span>
            {#if entry.platform}
              <ConsoleTag platform={entry.platform} />
            {:else if entry.source}
              <SourceBadge source={entry.source} />
            {/if}
            <span class="list-meta">{entry.subtitle}</span>
            <span class="list-meta right">{entry.watched ? '' : entryMeta(entry)}</span>
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
