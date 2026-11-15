import { get } from 'svelte/store';
import { createPagePrefetch } from '../../lib/catalog/prefetch';
import { loadCatalogContinuation, refreshCatalogSnapshot, reloadCatalogPrefix, withoutStale } from '../../lib/catalog/pages';
import { applyFreshPage, canWarmNextPage, createFreshPageBuffer, staleKeyOf, type FreshPageEvent } from '../../lib/catalog/stale';
import { identityEvidenceChanged, identityFingerprint, matchesCatalogIdentity } from '../../lib/catalog/identity';
import type { CatalogSort } from '../../lib/services/recommendations';
import {
  compatOnlyWorking,
  isCancelledRequest,
  queryCatalogGames,
  type CatalogGame,
  type CatalogQuery,
  type CompatInfo,
  type GenreFacet,
  type Source,
} from '../../lib/services/sources';
import { inWails } from '../../lib/services/backend';
import { sources } from '../../lib/stores/sources';
import { errorCode } from '../../lib/i18n';

const pageSize = 60;

/** What the loaded pages keep for the way back to the page. */
export interface CatalogPagesSnapshot {
  catalogFallback: boolean;
  snapshot: string;
  sourceState: Source[] | undefined;
  items: CatalogGame[];
  total: number;
  page: number;
  failed: boolean;
  revision: number;
  offline: boolean;
  incomplete: boolean;
  facets: GenreFacet[];
  platforms: GenreFacet[];
}

/** The query the pages are asked with: read through getters, so each read is the page's current choice. */
export interface CatalogPagesQuery {
  readonly search: string;
  readonly genre: string;
  readonly platform: string;
  readonly kind: string;
  readonly sort: CatalogSort;
  readonly compatOnly: boolean;
  readonly hideLibrary: boolean;
  readonly hideNotInterested: boolean;
  /** The page's reload, asked for by a fresh page that cannot replace the shown one. */
  reload(refreshProfile?: boolean): Promise<void>;
}

export type CatalogPages = ReturnType<typeof createCatalogPages>;

/**
 * The catalog's loaded pages: the first page and its continuations, the prefetch of the next one,
 * the fresh pages that replace a stale one, the refresh of the loaded prefix when a game's identity
 * changes; and the tokens that drop a late answer. Called once by the page's data.
 */
export function createCatalogPages(restored: CatalogPagesSnapshot | undefined, view: CatalogPagesQuery) {
  let sourceState = restored?.sourceState;
  let snapshot = $state(restored?.snapshot ?? '');

  let compatByGame = $state<Record<string, CompatInfo>>({});
  let items = $state<CatalogGame[]>(restored?.items ?? []);
  let total = $state(restored?.total ?? 0);
  let page = $state(restored?.page ?? 0);
  let loading = $state(!restored);
  let appending = $state(false);
  let autoPages = $state(0);
  let failed = $state(restored?.failed ?? false);
  let backendOutdated = $state(false);
  let revision = $state(restored?.revision ?? 0);
  let offline = $state(restored?.offline ?? false);
  let incomplete = $state(restored?.incomplete ?? false);
  let facets = $state<GenreFacet[]>(restored?.facets ?? []);
  let platforms = $state<GenreFacet[]>(restored?.platforms ?? []);
  let catalogFallback = $state(restored?.catalogFallback ?? false);

  function personalQuery() {
    return { stable: true, snapshot, hideLibrary: view.hideLibrary, hideNotInterested: view.hideNotInterested };
  }

  let staleKey = '';
  const earlyFresh = createFreshPageBuffer();
  let token = 0;
  let catalogAbort = new AbortController();
  let identityRefreshKey = '';
  let identityRefreshRunning = false;
  let identityRefreshToken = 0;
  // Browse responses already contain list artwork. Loading full metadata for
  // every visible card also changes provider identities while paging through
  // the catalog; reserve that work for the game detail view.

  const prefetch = createPagePrefetch<Awaited<ReturnType<typeof queryCatalogGames>>>();

  async function fetchPage(next: number) {
    const current = ++token;
    const requestedSources = get(sources);
    const load = (q: CatalogQuery) => queryCatalogGames(q, catalogAbort.signal);
    loading = true;
    appending = next > 1;
    if (next === 1) autoPages = 0;
    try {
      const request = {
        ...personalQuery(),
        revision: next === 1 ? 0 : revision,
        search: view.search,
        genre: view.genre, platform: view.platform, kind: view.kind,
        sort: view.sort,
        compat: view.compatOnly ? compatOnlyWorking : '',
        page: next,
        pageSize,
        ...(next === 1 ? { allowStale: true } : {}),
      };
      const continuation = await loadCatalogContinuation(
        request, items, load,
        () => prefetch.take(JSON.stringify(request), () => load(request)),
        () => current === token,
        offline,
      );
      const { result, refreshed } = continuation;
      if (current !== token) return;
      sourceState = requestedSources;
      items = continuation.items;
      compatByGame = next === 1 || refreshed ? (result.compat ?? {}) : { ...compatByGame, ...(result.compat ?? {}) };
      total = result.total;
      page = result.page;
      failed = false;
      backendOutdated = false;
      facets = result.facets ?? [];
      platforms = result.platforms ?? [];
      revision = result.revision ?? 0;
      snapshot = result.snapshot ?? '';
      if (next === 1 || refreshed) catalogFallback = result.personalizationFallback ?? false;
      else catalogFallback ||= result.personalizationFallback ?? false;
      offline = result.offline ?? false;
      incomplete = !result.providers?.length || result.providers.some((p) => !p.complete);
      if (next === 1 || refreshed) staleKey = staleKeyOf(result);
      if (canWarmNextPage({ offline, staleKey, loaded: items.length, total })) {
        const upcoming = { ...withoutStale(request), page: page + 1, revision, snapshot };
        prefetch.warm(JSON.stringify(upcoming), () => load(upcoming));
      }
      applyEarlyFreshPage();
    } catch (err) {
      if (current !== token || isCancelledRequest(err)) return;
      backendOutdated = errorCode(err) === "catalog.backend_outdated";
      prefetch.clear();
      if (next === 1) {
        staleKey = '';
        items = [];
        total = 0;
        page = 0;
        revision = 0;
        offline = false;
        incomplete = false;
        facets = [];
        platforms = [];
        compatByGame = {};
      }
      failed = true;
    } finally {
      if (current === token) {
        loading = false;
        appending = false;
      }
    }
  }

  // The page's reload drops what is loaded and every answer still on its way, before it asks again.
  function reset() {
    token++;
    catalogAbort.abort();
    catalogAbort = new AbortController();
    identityRefreshRunning = false;
    identityRefreshToken = 0;
    prefetch.clear();
    staleKey = '';
    earlyFresh.clear();
    loading = true;
    items = [];
    total = 0;
    page = 0;
    revision = 0;
    snapshot = '';
    catalogFallback = false;
  }

  async function refreshLoadedPrefix(validateSnapshot = false) {
    if (!inWails || identityRefreshRunning || page <= 0) return;
    const current = ++token;
    identityRefreshToken = current;
    const targetPage = page;
    identityRefreshRunning = true;
    loading = true;
    appending = false;
    prefetch.clear();
    const load = (q: CatalogQuery) => queryCatalogGames(q, catalogAbort.signal);
    try {
      const request = {
        ...personalQuery(), revision, search: view.search, genre: view.genre, platform: view.platform, kind: view.kind, sort: view.sort,
        compat: view.compatOnly ? compatOnlyWorking : '',
        page: targetPage, pageSize,
      };
      const prefix = validateSnapshot
        ? await refreshCatalogSnapshot(request, items, compatByGame, load, () => current === token, offline)
        : await reloadCatalogPrefix(request, targetPage, load, () => current === token);
      if (current !== token) return;
      sourceState = get(sources);
      items = prefix.items;
      compatByGame = prefix.compat;
      total = prefix.result.total;
      page = prefix.result.page;
      failed = false;
      backendOutdated = false;
      facets = prefix.result.facets ?? [];
      platforms = prefix.result.platforms ?? [];
      revision = prefix.result.revision ?? 0;
      snapshot = prefix.result.snapshot ?? '';
      catalogFallback ||= prefix.result.personalizationFallback ?? false;
      offline = prefix.result.offline ?? false;
      incomplete = !prefix.result.providers?.length || prefix.result.providers.some((p) => !p.complete);
      staleKey = staleKeyOf(prefix.result);
      if (canWarmNextPage({ offline, staleKey, loaded: items.length, total })) {
        const upcoming = {
          ...personalQuery(), revision,
          search: view.search,
          genre: view.genre,
          platform: view.platform,
          kind: view.kind,
          sort: view.sort,
          compat: view.compatOnly ? compatOnlyWorking : '',
          page: page + 1,
          pageSize,
        };
        prefetch.warm(JSON.stringify(upcoming), () => load(upcoming));
      }
      applyEarlyFreshPage();
    } catch {
      if (current === token) prefetch.clear();
    } finally {
      if (identityRefreshToken === current) {
        identityRefreshRunning = false;
        if (current === token) {
          loading = false;
          appending = false;
        }
      }
    }
  }

  function refreshForMetadata(game: CatalogGame | undefined) {
    if (!game || page <= 0) return;
    const item = items.find((candidate) => matchesCatalogIdentity(candidate, game));
    if (!item || !identityEvidenceChanged(item, game)) return;
    const key = `${item.id}:${identityFingerprint(game)}`;
    if (key === identityRefreshKey) return;
    identityRefreshKey = key;
    void refreshLoadedPrefix();
  }

  function applyEarlyFreshPage() {
    if (!staleKey) return;
    const early = earlyFresh.take(staleKey);
    if (early) onFreshPage(early);
  }

  function onFreshPage(event: FreshPageEvent | undefined) {
    if (event?.key && event.key !== staleKey) {
      earlyFresh.remember(event);
      return;
    }
    const verdict = applyFreshPage(staleKey, { revision, page: appending ? page + 1 : page, fallback: catalogFallback }, event);
    if (verdict.action === 'ignore') {
      if (event?.key && event.key === staleKey) {
        staleKey = '';
        earlyFresh.take(event.key);
      }
      return;
    }
    earlyFresh.take(staleKey);
    staleKey = '';
    if (verdict.action === 'reload') {
      void view.reload(false);
      return;
    }
    if (verdict.action === 'failed') {
      if (verdict.error === 'catalog.changed') void view.reload(false);
      else if (verdict.error === 'catalog.backend_outdated') backendOutdated = true;
      else offline = true;
      return;
    }
    const fresh = verdict.page;
    items = fresh.items ?? [];
    compatByGame = fresh.compat ?? {};
    total = fresh.total;
    facets = fresh.facets ?? [];
    platforms = fresh.platforms ?? [];
    catalogFallback = fresh.personalizationFallback ?? false;
    offline = fresh.offline ?? false;
    incomplete = !fresh.providers?.length || fresh.providers.some((p) => !p.complete);
    if (canWarmNextPage({ offline, staleKey, loaded: items.length, total })) {
      const upcoming = {
        ...personalQuery(), revision, search: view.search, genre: view.genre, platform: view.platform, kind: view.kind, sort: view.sort,
        compat: view.compatOnly ? compatOnlyWorking : '',
        page: page + 1, pageSize,
      };
      prefetch.warm(JSON.stringify(upcoming), () => queryCatalogGames(upcoming, catalogAbort.signal));
    }
  }

  return {
    get sourceState() { return sourceState; },
    get snapshot() { return snapshot; },
    get compatByGame() { return compatByGame; },
    get items() { return items; },
    get total() { return total; },
    get page() { return page; },
    get loading() { return loading; },
    get appending() { return appending; },
    get autoPages() { return autoPages; },
    set autoPages(count: number) { autoPages = count; },
    get failed() { return failed; },
    get backendOutdated() { return backendOutdated; },
    get revision() { return revision; },
    get offline() { return offline; },
    get incomplete() { return incomplete; },
    get facets() { return facets; },
    get platforms() { return platforms; },
    get catalogFallback() { return catalogFallback; },
    get token() { return token; },
    set token(value: number) { token = value; },
    get catalogAbort() { return catalogAbort; },
    fetchPage,
    reset,
    refreshLoadedPrefix,
    refreshForMetadata,
    onFreshPage,
  };
}
