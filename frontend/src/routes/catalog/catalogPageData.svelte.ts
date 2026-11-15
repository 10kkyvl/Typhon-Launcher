import { onDestroy, onMount, tick } from 'svelte';
import { get } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { catalogWithoutDiscovery } from '../../lib/catalog/display';
import { nextGenre } from '../../lib/catalog/filters';
import { latestRunner, serialQueue } from '../../lib/catalog/serial';
import type { FreshPageEvent } from '../../lib/catalog/stale';
import { maxAutoPages, observerAvailable, shouldLoadMore, watchSentinel } from '../../lib/catalog/infinite';
import type { Recommendation } from '../../lib/recommendations/display';
import { discoveryCache } from '../../lib/recommendations/cache';
import { defaultPreferences, emptyProfile, getDiscovery, getRecommendationPreferences, getRecommendationProfile, saveRecommendationPreferences, setNotInterested, type CatalogSort } from '../../lib/services/recommendations';
import { setFavorite } from '../../lib/services/library';
import { compatOnlyWorking, type CatalogGame } from '../../lib/services/sources';
import { inWails } from '../../lib/services/backend';
import { getAppInfo } from '../../lib/services/system';
import type { MetadataView } from '../../lib/services/metadata';
import { libraryGames } from '../../lib/stores/library';
import { currentRouteKey, recallRoute, route, stashRoute } from '../../lib/stores/router';
import { toast } from '../../lib/stores/toasts';
import { errorCode, hasMessage, msg } from '../../lib/i18n';
import { focusableIn, focusQuietly } from '../../lib/layers/focus';
import { createCatalogPages, type CatalogPagesSnapshot } from './catalogPages.svelte';

export function libraryErrorText(err: unknown, fallback: string): string {
  const code = errorCode(err);
  return hasMessage(code) ? msg(code) : fallback;
}

export type Sort = CatalogSort;

interface Snapshot extends CatalogPagesSnapshot {
  preferenceKey: string;
  discoveryFallback: boolean;
  discoveryStale: boolean;
  profile: ReturnType<typeof emptyProfile>;
  discovery: Recommendation[];
  hideLibrary: boolean;
  hideNotInterested: boolean;
  search: string;
  genre: string;
  sort: Sort;
  compatOnly: boolean;
  platform: string;
  kind: string;
}

export type CatalogPageData = ReturnType<typeof createCatalogPageData>;

/**
 * The catalog page's data: the search and the filters, the saved preferences and the profile, the
 * recommendation shelf, the dismissals and their undo, the retries and the paging, with the page's
 * subscriptions and its state kept for the way back; the loaded pages are `pages`. Called once while
 * the page initialises: its effects are the page's, created in the page's order.
 */
export function createCatalogPageData() {
  const allGenres = msg('games.filterAll');
  const sortLabels: Record<Sort, string> = {
    title: msg('games.sortAlpha'),
    year: msg('games.catalogSortNewest'),
    popular: msg('games.catalogSortPopular'),
    rating: msg('games.catalogSortRating'),
    'for-you': msg('games.catalogSortForYou'),
    auto: msg('games.catalogSortAuto'),
  };

  const routeKey = currentRouteKey();
  const restored = recallRoute<Snapshot>(routeKey, 'catalog');

  let search = $state(restored?.search ?? get(route).params.q ?? '');
  let genre = $state(restored?.genre ?? '');
  let sort = $state<Sort>(restored?.sort ?? 'auto');
  let compatOnly = $state(restored?.compatOnly ?? false);
  // Бейдж и фильтр имеют смысл только там, где игры идут через CrossOver: на
  // Windows они запускаются нативно, и цифра «94% запускается» там ни о чём.
  let compatRelevant = $state(false);
  let sentinel = $state<HTMLElement>();
  let platform = $state(restored?.platform ?? "");
  let kind = $state(restored?.kind ?? "");

  let preferenceKey = restored?.preferenceKey ?? '';
  let preferences = $state(defaultPreferences());
  let profile = $state(restored?.profile ?? emptyProfile());
  let hideLibrary = $state(restored?.hideLibrary ?? false);
  let hideNotInterested = $state(restored?.hideNotInterested ?? true);
  let discovery = $state<Recommendation[]>(restored?.discovery ?? []);
  let discoveryLoading = $state(false);
  let discoveryFallback = $state(restored?.discoveryFallback ?? false);
  let discoveryStale = $state(restored?.discoveryStale ?? false);

  const pages = createCatalogPages(restored, {
    get search() { return search; },
    get genre() { return genre; },
    get platform() { return platform; },
    get kind() { return kind; },
    get sort() { return sort; },
    get compatOnly() { return compatOnly; },
    get hideLibrary() { return hideLibrary; },
    get hideNotInterested() { return hideNotInterested; },
    reload,
  });

  const personalizationFallback = $derived(pages.catalogFallback || (discoveryFallback && !discoveryStale));
  let lastDismissed = $state<{ id: string; title: string } | null>(null);
  let preferenceBusy = $state(false);
  let ready = $state(false);
  let reloadToken = 0;
  let favoriteSeq = 0;
  const effectiveSort = $derived(sort === 'auto' ? profile.defaultSort : sort);
  const discoveryVisible = $derived(effectiveSort === 'for-you' && !search.trim());
  const displayedItems = $derived(catalogWithoutDiscovery(pages.items, discoveryVisible ? discovery : []));

  function favoriteViewKey(): string {
    return JSON.stringify({ search, genre, platform, kind, sort, compatOnly });
  }

  let debounce: ReturnType<typeof setTimeout> | undefined;

  onDestroy(() => {
    clearTimeout(debounce);
    pages.token++;
    reloadToken++;
    pages.catalogAbort.abort();
    stashRoute(routeKey, 'catalog', {
      revision: pages.revision, offline: pages.offline, incomplete: pages.incomplete, platform, kind, facets: pages.facets, platforms: pages.platforms,
      sourceState: pages.sourceState, snapshot: pages.snapshot, preferenceKey, catalogFallback: pages.catalogFallback, discoveryFallback, discoveryStale, profile, discovery: [...discovery], hideLibrary, hideNotInterested,
      search,
      genre,
      sort,
      compatOnly,
      items: [...pages.items],
      total: pages.total,
      page: pages.page,
      failed: pages.failed,
    } satisfies Snapshot);
  });

  const chips = $derived([allGenres, ...pages.facets.filter((f) => f.count > 0).map((f) => f.label)]);

  async function reload(refreshProfile = true) {
    if (!ready) return;
    const active = ++reloadToken;
    pages.reset();
    // Pages own their frozen ranking and membership. The shelf may finish or
    // revalidate later without changing the pagination request or its offsets.
    await Promise.all([pages.fetchPage(1), refreshDiscovery(active, { refreshProfile })]);
  }

  async function refreshDiscovery(active: number, { rotate = false, refreshProfile = false } = {}) {
    const query = { search, genre, platform, kind, sort, compat: compatOnly ? compatOnlyWorking : '' };
    const previousIDs = rotate ? discovery.map((item) => item.game.id) : [];
    discoveryLoading = true;
    if (refreshProfile) {
      try {
        const currentProfile = await getRecommendationProfile();
        if (active !== reloadToken) return;
        profile = currentProfile;
      } catch {
        if (active !== reloadToken) return;
        discoveryFallback = true;
      }
    }
    preferenceKey = JSON.stringify({ preferences, profile });
    if (discoveryVisible) {
      const key = JSON.stringify({ query, preferences, profile, library: get(libraryGames).map((game) => [
        game.id, game.canonicalGameId, game.favorite, game.status, game.uninstalled, game.playtimeSeconds, game.lastPlayed,
      ]) });
      const { cached, refreshed } = discoveryCache.get(key, () => getDiscovery(query, previousIDs), rotate);
      discovery = cached?.items ?? [];
      discoveryFallback = cached?.fallback ?? false;
      discoveryStale = false;
      try {
        if (refreshed) {
          const result = await refreshed;
          if (active !== reloadToken) return;
          discovery = result.items;
          discoveryFallback = result.fallback;
          discoveryStale = result.fallback && Boolean(cached && !cached.fallback);
        }
      } catch {
        if (active !== reloadToken) return;
        discoveryFallback = true;
        discoveryStale = Boolean(cached);
      } finally {
        if (active === reloadToken) discoveryLoading = false;
      }
    } else {
      discovery = [];
      discoveryLoading = false;
      discoveryFallback = false;
      discoveryStale = false;
    }
  }

  // The first page failed: nothing of the catalog is on screen. Said in the place of the offline
  // notice, by the same notice, so a retry that answers with the other state keeps its focused Retry.
  // A new query (search, filter, sort) is not that retry: it shows loading, not the old failure.
  let pageRetrying = $state(false);
  const pageFailed = $derived(pages.failed && pages.items.length === 0 && (!pages.loading || pageRetrying));
  let results = $state<HTMLElement>();

  // The notice's retry: it worked when the catalog came back, online.
  async function retryFirstPage(): Promise<boolean> {
    await reload();
    return !pages.failed && !pages.offline;
  }

  let moreSlot = $state<HTMLElement>();
  let moreRetrying = $state(false);
  let freshFrom = 0;

  // "Show more" goes when its page fails and the failure takes its place: focus that was on it moves
  // to that notice's Retry instead of falling to the page. A refusal has no Retry: the notice itself
  // takes the focus there, so the reason is read where the pressed button was and Tab goes on from it.
  async function showMore() {
    const pressed = Boolean(moreSlot?.contains(document.activeElement));
    pages.autoPages = 0;
    await pages.fetchPage(pages.page + 1);
    if (!pressed || !pages.failed) return;
    await tick();
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected) return;
    const target = moreSlot ? (focusableIn(moreSlot)[0] ?? moreSlot.querySelector<HTMLElement>('[data-more-refused]')) : undefined;
    if (target) focusQuietly(target);
  }

  // The notice's retry of the page that failed: on success focus goes to the first game it brought.
  async function retryNextPage(): Promise<boolean> {
    freshFrom = displayedItems.length;
    await pages.fetchPage(pages.page + 1);
    return !pages.failed;
  }

  function firstFresh(): HTMLElement | undefined {
    const node = results?.children[freshFrom];
    return node instanceof HTMLElement ? focusableIn(node)[0] : undefined;
  }

  function refreshPicks() {
    if (!ready || discoveryLoading) return;
    void refreshDiscovery(++reloadToken, { rotate: true });
  }

  function restoreChoices() {
    sort = (preferences.defaultSort || 'auto') as Sort;
    genre = preferences.genre;
    platform = preferences.platform;
    kind = preferences.kind;
    compatOnly = preferences.compatOnly;
    hideLibrary = preferences.hideLibrary;
    hideNotInterested = preferences.hideNotInterested;
  }

  const preferenceWrites = serialQueue();

  const applyPreferences = latestRunner(() => preferenceWrites(async () => {
    preferenceBusy = true;
    try {
      const next = { ...preferences, defaultSort: sort === 'auto' ? '' : sort, genre,
        platform, kind, compatOnly, hideLibrary, hideNotInterested };
      await saveRecommendationPreferences(next);
      preferences = next;
      preferenceKey = JSON.stringify({ preferences, profile });
      await reload();
    } catch {
      restoreChoices();
      toast(msg('games.recommendationError'), 'danger');
    } finally { preferenceBusy = false; }
  }));

  function preferencesChanged() {
    return applyPreferences();
  }

  function isDismissed(game: CatalogGame) {
    return [game.id, game.serverId, ...(game.aliasIds ?? [])].some((id) => id && preferences.notInterested.includes(id));
  }

  // `changed` runs once the choice is saved and shown, before the catalog reloads with it.
  function dismiss(game: CatalogGame, on = true, changed?: () => void) {
    return preferenceWrites(async () => {
      preferenceBusy = true;
      try {
        await setNotInterested(game.id, on);
        preferences = await getRecommendationPreferences();
        lastDismissed = on ? { id: game.id, title: game.title } : null;
        changed?.();
        await reload();
      } catch { toast(msg('games.recommendationError'), 'danger'); }
      finally { preferenceBusy = false; }
    });
  }

  let undoing = $state(false);
  // Where the put-away notice sat: the control after it takes the focus when the notice goes, else the
  // one before it (the list after it may be reloading, with nothing to focus yet).
  let dismissalEnd = $state<HTMLElement>();

  async function undoDismissal() {
    if (!lastDismissed || undoing) return;
    undoing = true;
    try {
      await dismiss({ id: lastDismissed.id, title: lastDismissed.title } as CatalogGame, false, () => void focusPastDismissal());
    } finally {
      undoing = false;
    }
  }

  async function focusPastDismissal() {
    await tick();
    const active = document.activeElement;
    const end = dismissalEnd;
    if (!end || (active && active !== document.body && active.isConnected)) return;
    const all = focusableIn(document.body);
    const after = all.find((el) => end.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING);
    const before = all.findLast((el) => end.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_PRECEDING);
    const next = after ?? before;
    if (next) focusQuietly(next);
  }

  function onSearch() {
    clearTimeout(debounce);
    debounce = setTimeout(() => void reload(false), 350);
  }

  function onSort(value: Sort) {
    if (value === sort) return;
    sort = value;
    void preferencesChanged();
  }

  function onGenre(label: string) {
    const value = label === allGenres ? '' : label;
    const next = nextGenre(genre, value);
    if (next === genre) return;
    genre = next;
    void preferencesChanged();
  }

  function onCompatOnly() {
    compatOnly = !compatOnly;
    void preferencesChanged();
  }

  function onPlatform(id: string) {
    platform = id;
    void preferencesChanged();
  }

  function onKind(id: string) {
    kind = id;
    void preferencesChanged();
  }

  function onHideLibrary() {
    hideLibrary = !hideLibrary;
    void preferencesChanged();
  }

  function onHideNotInterested() {
    hideNotInterested = !hideNotInterested;
    void preferencesChanged();
  }

  onMount(() => {
    let active = true;
    void (async () => {
      try {
        const [saved, currentProfile] = await Promise.all([getRecommendationPreferences(), getRecommendationProfile()]);
        if (!active) return;
        preferences = saved;
        profile = currentProfile;
        restoreChoices();
      } catch { discoveryFallback = true; }
      if (!active) return;
      ready = true;
      const nextKey = JSON.stringify({ preferences, profile });
      const reuse = restored && pages.page > 0 && preferenceKey === nextKey;
      preferenceKey = nextKey;
      if (reuse) await Promise.all([pages.refreshLoadedPrefix(true), refreshDiscovery(reloadToken)]);
      else await reload(false);
    })();
    return () => { active = false; ready = false; };
  });

  onMount(() => {
    if (!inWails) return;
    return Events.On('metadata:updated', (event) => {
      pages.refreshForMetadata((event.data as MetadataView)?.game);
    });
  });

  onMount(() => {
    if (!inWails) return;
    return Events.On('catalog:page', (event) => {
      pages.onFreshPage(event.data as FreshPageEvent | undefined);
    });
  });

  onMount(async () => {
    try { compatRelevant = (await getAppInfo()).platform === 'darwin'; }
    catch { compatRelevant = false; }
  });

  const hasMore = $derived(pages.items.length < pages.total);
  const autoLoad = observerAvailable();
  const showMoreButton = $derived(hasMore && (!autoLoad || pages.failed || pages.autoPages >= maxAutoPages));

  $effect(() => {
    if (!sentinel || !autoLoad) return;
    if (!shouldLoadMore({ hasMore, loading: pages.loading, failed: pages.failed, pagesSinceScroll: pages.autoPages })) return;
    return watchSentinel(sentinel, () => {
      pages.autoPages += 1;
      void pages.fetchPage(pages.page + 1);
    });
  });

  onMount(() => {
    const resetAutoPages = () => { if (pages.autoPages) pages.autoPages = 0; };
    document.addEventListener('scroll', resetAutoPages, { capture: true, passive: true });
    return () => document.removeEventListener('scroll', resetAutoPages, true);
  });

  async function toggleFavorite(libraryId: string, current: boolean) {
    const expected = {
      favorite: ++favoriteSeq,
      reload: reloadToken,
      token: pages.token,
      view: favoriteViewKey(),
    };
    const isCurrent = () => expected.favorite === favoriteSeq && expected.reload === reloadToken
      && expected.token === pages.token && expected.view === favoriteViewKey();
    try {
      await setFavorite(libraryId, !current);
      // Saving a heart updates the library store immediately. Keep the visible
      // shelf and loaded pages frozen until an explicit refresh/filter change.
    } catch (err) {
      if (isCurrent()) toast(libraryErrorText(err, msg('games.errorFavoriteFailed')), 'danger');
    }
  }

  return {
    pages,
    allGenres,
    sortLabels,
    get search() { return search; },
    set search(text: string) { search = text; },
    get genre() { return genre; },
    get sort() { return sort; },
    get compatOnly() { return compatOnly; },
    get compatRelevant() { return compatRelevant; },
    get sentinel() { return sentinel; },
    set sentinel(node: HTMLElement | undefined) { sentinel = node; },
    get platform() { return platform; },
    get kind() { return kind; },
    get profile() { return profile; },
    get hideLibrary() { return hideLibrary; },
    get hideNotInterested() { return hideNotInterested; },
    get discovery() { return discovery; },
    get discoveryLoading() { return discoveryLoading; },
    get discoveryStale() { return discoveryStale; },
    get personalizationFallback() { return personalizationFallback; },
    get lastDismissed() { return lastDismissed; },
    get preferenceBusy() { return preferenceBusy; },
    get ready() { return ready; },
    get effectiveSort() { return effectiveSort; },
    get discoveryVisible() { return discoveryVisible; },
    get displayedItems() { return displayedItems; },
    get chips() { return chips; },
    get pageRetrying() { return pageRetrying; },
    set pageRetrying(on: boolean) { pageRetrying = on; },
    get pageFailed() { return pageFailed; },
    get results() { return results; },
    set results(node: HTMLElement | undefined) { results = node; },
    get moreSlot() { return moreSlot; },
    set moreSlot(node: HTMLElement | undefined) { moreSlot = node; },
    get moreRetrying() { return moreRetrying; },
    set moreRetrying(on: boolean) { moreRetrying = on; },
    get undoing() { return undoing; },
    get dismissalEnd() { return dismissalEnd; },
    set dismissalEnd(node: HTMLElement | undefined) { dismissalEnd = node; },
    get hasMore() { return hasMore; },
    autoLoad,
    get showMoreButton() { return showMoreButton; },
    retryFirstPage,
    showMore,
    retryNextPage,
    firstFresh,
    refreshPicks,
    isDismissed,
    dismiss,
    undoDismissal,
    onSearch,
    onSort,
    onGenre,
    onCompatOnly,
    onPlatform,
    onKind,
    onHideLibrary,
    onHideNotInterested,
    toggleFavorite,
  };
}
