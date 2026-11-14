import { onMount, tick, untrack } from 'svelte';
import { Events } from '@wailsio/runtime';
import { metaStatus, preferView } from '../../lib/game/view';
import {
  addCatalogGame,
  createShortcut,
  findEmulatedCover,
  markError,
  removeShortcut,
  setExecutable,
  setFavorite,
  setRequiresSteam,
  type LibraryGame,
} from '../../lib/services/library';
import { selectImageFile } from '../../lib/services/emulators';
import { refreshMissingImages } from '../../lib/stores/missingImages';
import {
  dismissMetadataMatch,
  ensureMetadataFresh,
  getMetadataView,
  getSavePaths,
  refreshMetadata,
  type MetadataView,
  type SavePaths,
} from '../../lib/services/metadata';
import {
  getCatalogGame,
  getRelatedReleases,
  getReleasesForGame,
  getReleasesForTitle,
  type RelatedReleases,
  type CatalogGame,
  type ReleaseGroup,
  type Source,
} from '../../lib/services/sources';
import { releaseSourcesKey } from '../../lib/game/releases';
import { getWineStatus, type WineStatus } from '../../lib/services/system';
import { toast } from '../../lib/stores/toasts';
import { errorCode, hasMessage, msg } from '../../lib/i18n';
import { metadataErrorText } from '../../lib/metadata/metadataErrors';
import { focusQuietly } from '../../lib/layers/focus';

export function libraryErrorText(err: unknown, fallback: string): string {
  const code = errorCode(err);
  return hasMessage(code) ? msg(code) : fallback;
}

export type GamePageData = ReturnType<typeof createGamePageData>;

/** What the page's data follows: read through getters, so each read is the page's current value. */
export interface GamePageInputs {
  readonly id: string;
  readonly catalogKey: string;
  readonly canonicalId: string | undefined;
  readonly locale: string;
  readonly sources: readonly Source[];
  readonly metadataAvailable: boolean;
  readonly releaseKey: string;
  readonly releaseTitle: string | undefined;
  readonly title: string;
  readonly coverSrc: string;
  readonly external: boolean;
  readonly sideloaded: boolean;
  readonly localGame: LibraryGame | undefined;
  readonly emulatedPlatform: string;
  readonly titleHeading: HTMLElement | undefined;
}

/**
 * The game page's data: the catalog entry, the metadata view, the releases and other versions, the
 * save paths and the Wine status, with the loaders and the `metadata:updated` subscription; and the
 * page's writes to the library card. Called once while the page initialises: its effects are the
 * page's, created in the page's order.
 */
export function createGamePageData(page: GamePageInputs) {
  let requiresSteam = $state(true);
  let requiresSteamSaving = $state(false);

  $effect(() => {
    if (!requiresSteamSaving) requiresSteam = page.localGame?.requiresSteam !== false;
  });

  let releaseGroups = $state<ReleaseGroup[]>([]);
  let releasesLoading = $state(false);
  let releasesFailed = $state(false);
  let releaseToken = 0;

  let related = $state<RelatedReleases[]>([]);
  let relatedLoading = $state(false);
  let relatedFailed = $state(false);
  let relatedLoaded = $state(false);
  let relatedToken = 0;

  // A feed can finish importing while this game is already open; a refresh that changed nothing the
  // releases depend on sends the sources again and must not ask again.
  const sourcesKey = $derived(releaseSourcesKey(page.sources));
  let loadedReleaseKey: string | null = null;

  let catalogGame = $state<CatalogGame | null>(null);
  let catalogLoading = $state(false);
  let catalogFailed = $state(false);
  let catalogRetrying = $state(false);
  let catalogToken = 0;

  async function loadCatalogGame(gameId: string) {
    const current = ++catalogToken;
    catalogLoading = true;
    catalogFailed = false;
    catalogGame = null;
    try {
      const found = await getCatalogGame(gameId);
      if (current !== catalogToken) return;
      catalogGame = found;
    } catch (err) {
      if (current !== catalogToken) return;
      catalogFailed = true;
      toast(metadataErrorText(err, msg('games.detailCatalogLoadError')), 'danger');
    } finally {
      if (current === catalogToken) catalogLoading = false;
    }
  }

  $effect(() => {
    const gameId = page.catalogKey;
    untrack(() => {
      if (!gameId) {
        catalogToken++;
        catalogLoading = false;
        catalogFailed = false;
        catalogGame = null;
        return;
      }
      loadCatalogGame(gameId);
    });
  });

  let metaView = $state<MetadataView | null>(null);
  let metaToken = 0;
  let metaEventVersion = 0;
  let metaReading = $state(false);
  let metaRefreshing = $state(false);
  let metaSearching = $state(false);
  let metaSkipping = $state(false);
  // A search ran while the page was open: its outcome is news, not a standing fact of the page.
  let metaSearchRan = $state(false);
  let wineStatus = $state<WineStatus | null>(null);
  // The view itself could not be read: the failed notice says so, with a Retry that reads it again,
  // instead of a page that looks as if the game had no metadata.
  let metaReadFailed = $state(false);

  async function loadMetaView(gameId: string) {
    const current = ++metaToken;
    const eventVersion = metaEventVersion;
    metaReading = true;
    try {
      let view: MetadataView;
      try {
        view = await getMetadataView(gameId);
      } catch (err) {
        if (current !== metaToken) return;
        metaReadFailed = true;
        metaFailure = metadataErrorText(err, msg('games.detailMetaLoadError'));
        return;
      }
      if (current !== metaToken) return;
      metaReadFailed = false;
      if (eventVersion === metaEventVersion) metaView = preferView(metaView, view);
      const started = await ensureMetadataFresh(gameId);
      if (current !== metaToken || eventVersion !== metaEventVersion) return;
      metaSearching = started || view.match === 'searching';
    } catch (err) {
      if (current === metaToken) toast(metadataErrorText(err, msg('games.detailMetaLoadError')), 'danger');
    } finally {
      if (current === metaToken) metaReading = false;
    }
  }

  $effect(() => {
    const metaGameId = page.canonicalId;
    void page.locale;
    untrack(() => {
      metaSearching = false;
      metaSearchRan = false;
      metaReadFailed = false;
      metaView = null;
      if (!metaGameId) {
        metaToken++;
        metaReading = false;
        metaView = null;
        return;
      }
      loadMetaView(metaGameId);
    });
  });

  onMount(async () => {
    try {
      wineStatus = await getWineStatus();
    } catch (err) {
      toast(markError(err, msg('games.errorWineStatusFailed')), 'danger');
    }
  });

  onMount(() => {
    return Events.On('metadata:updated', (event) => {
      const view = event.data as MetadataView;
      if (view.game?.id && view.game.id === page.canonicalId) {
        metaEventVersion++;
        metaSearching = view.match === 'searching';
        metaReadFailed = false;
        metaView = preferView(metaView, view);
      }
    });
  });

  // A retry from the failed state keeps that notice, with its busy Retry, until the answer arrives.
  let metaRetrying = $state(false);

  const metaState = $derived(
    metaReadFailed
      ? 'failed'
      : metaStatus({
          available: page.metadataAvailable,
          busy: metaReading || metaSearching || (metaRefreshing && !metaRetrying),
          match: metaView?.match,
          resolved: metaView?.resolved,
        }),
  );

  $effect(() => {
    if (metaSearching || metaView?.match === 'searching') metaSearchRan = true;
  });

  let savePaths = $state<SavePaths | null>(null);
  let savePathsToken = 0;

  $effect(() => {
    const gameId = page.canonicalId;
    untrack(() => {
      const current = ++savePathsToken;
      savePaths = null;
      if (!gameId) return;
      getSavePaths(gameId).then(
        (paths) => {
          if (current === savePathsToken) savePaths = paths;
        },
        () => {
          // a failed lookup only hides the optional PCGamingWiki link; the user sees no toast for it by design
          if (current === savePathsToken) savePaths = null;
        },
      );
    });
  });

  // Answers whether the metadata came back. From the failed notice its retry says a failure again
  // itself, so no toast doubles it.
  async function refreshMeta(fromNotice = false): Promise<boolean> {
    if (!page.canonicalId || metaRefreshing) return false;
    metaRefreshing = true;
    const gameId = page.canonicalId;
    const current = metaToken;
    try {
      const view = await refreshMetadata(gameId);
      if (gameId !== page.canonicalId || current !== metaToken) return false;
      metaView = view;
      metaReadFailed = false;
      // An answer can be a search that failed: from the notice its Retry says that again itself.
      if (view.match === 'failed') {
        if (!fromNotice) toast(msg('games.detailMetaRefreshError'), 'danger');
        return false;
      }
      toast(msg('games.detailMetaRefreshedToast'), 'success');
      return true;
    } catch (err) {
      if (!fromNotice) toast(metadataErrorText(err, msg('games.detailMetaRefreshError')), 'danger');
      else {
        metaFailure = metadataErrorText(err, msg('games.detailMetaFailedHint'));
        metaRetryable = !FINAL_META_FAILURES.has(errorCode(err));
      }
      return false;
    } finally {
      metaRefreshing = false;
    }
  }

  async function retryMetaRead(): Promise<boolean> {
    const gameId = page.canonicalId;
    if (!gameId) return false;
    await loadMetaView(gameId);
    return !metaReadFailed;
  }

  // Why the last retry from the failed notice failed, said in the notice in place of the generic
  // cause; and whether retrying can still help. Where it cannot, Retry goes and the focus that was on
  // it moves to "Choose manually", the way left.
  const FINAL_META_FAILURES = new Set(['metadata.no_match', 'metadata.ambiguous_match', 'metadata.not_configured']);
  let metaFailure = $state('');
  let metaRetryable = $state(true);

  $effect(() => {
    if (metaState === 'failed') return;
    metaFailure = '';
    metaRetryable = true;
  });

  // The page's error is an empty state, not a notice: when the game arrives it takes the focused
  // Retry away with it, and the focus goes to the game's title instead of falling to the page.
  async function retryCatalog() {
    if (catalogRetrying) return;
    const button = document.activeElement;
    catalogRetrying = true;
    try {
      await loadCatalogGame(page.id);
    } finally {
      catalogRetrying = false;
    }
    await tick();
    const active = document.activeElement;
    if (!button || button.isConnected || (active && active !== document.body && active.isConnected)) return;
    if (page.titleHeading?.isConnected) focusQuietly(page.titleHeading);
  }

  // Skipping keeps its notice, with Skip busy, until the answer; the notice then goes with the focused
  // Skip, and the focus goes to the game's title instead of the page.
  async function skipMeta() {
    if (!page.canonicalId || metaSkipping) return;
    const button = document.activeElement;
    metaSkipping = true;
    const gameId = page.canonicalId;
    const current = metaToken;
    try {
      const view = await dismissMetadataMatch(gameId);
      if (gameId !== page.canonicalId || current !== metaToken) return;
      metaView = view;
    } catch (err) {
      toast(metadataErrorText(err, msg('games.detailMetaSkipError')), 'danger');
    } finally {
      metaSkipping = false;
    }
    await tick();
    const active = document.activeElement;
    if (!button || button.isConnected || (active && active !== document.body && active.isConnected)) return;
    if (page.titleHeading?.isConnected) focusQuietly(page.titleHeading);
  }

  // `refresh`: the same game asked again because its sources changed. What is shown stays until the
  // answer, so the lists and the main action that reads them never pass through loading; a failed
  // refresh keeps the last answer.
  async function loadReleases(canonicalGameId: string | undefined, gameTitle: string | undefined, refresh = false) {
    const current = ++releaseToken;
    if (!refresh) {
      releasesLoading = true;
      releasesFailed = false;
      releaseGroups = [];
    }
    try {
      const groups = canonicalGameId
        ? await getReleasesForGame(canonicalGameId)
        : gameTitle
          ? await getReleasesForTitle(gameTitle)
          : [];
      if (current !== releaseToken) return;
      releaseGroups = groups;
      releasesFailed = false;
    } catch {
      if (current !== releaseToken || refresh) return;
      releasesFailed = true;
    } finally {
      if (current === releaseToken) releasesLoading = false;
    }
  }

  async function loadRelated(canonicalGameId: string | undefined, refresh = false) {
    const current = ++relatedToken;
    if (!refresh) {
      related = [];
      relatedFailed = false;
      relatedLoaded = false;
    }
    if (!canonicalGameId) {
      relatedLoading = false;
      return;
    }
    if (!refresh) relatedLoading = true;
    try {
      const list = await getRelatedReleases(canonicalGameId);
      if (current !== relatedToken) return;
      related = list;
      relatedFailed = false;
      relatedLoaded = true;
    } catch {
      if (current !== relatedToken || refresh) return;
      relatedFailed = true;
    } finally {
      if (current === relatedToken) relatedLoading = false;
    }
  }

  $effect(() => {
    const key = page.releaseKey;
    void sourcesKey;
    untrack(() => {
      const refresh = key === loadedReleaseKey;
      loadedReleaseKey = key;
      const canonicalGameId = page.canonicalId;
      const gameTitle = page.releaseTitle;
      const isExternalGame = page.external;
      if (page.sideloaded) {
        releaseToken++;
        relatedToken++;
        releaseGroups = [];
        releasesLoading = false;
        releasesFailed = false;
        relatedLoading = false;
        return;
      }
      loadRelated(isExternalGame ? undefined : canonicalGameId, refresh);
      loadReleases(canonicalGameId, gameTitle, refresh);
    });
  });

  async function retryReleases(): Promise<boolean> {
    await loadReleases(page.canonicalId, page.releaseTitle);
    return !releasesFailed;
  }

  async function retryRelated(): Promise<boolean> {
    await loadRelated(page.canonicalId);
    return !relatedFailed;
  }

  let imageBusy = $state(false);

  async function chooseImage() {
    if (!page.localGame || imageBusy) return;
    imageBusy = true;
    try {
      const path = await selectImageFile(msg('emul.addImageDialog'), [page.emulatedPlatform]);
      if (!path) return;
      await setExecutable(page.localGame.id, path);
      await refreshMissingImages();
      toast(msg('emul.imageSaved'), 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.imageSaveFailed')), 'danger');
    } finally {
      imageBusy = false;
    }
  }

  async function findCover() {
    if (!page.localGame || imageBusy) return;
    imageBusy = true;
    try {
      await findEmulatedCover(page.localGame.id);
      toast(msg('emul.coverFound'), 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.coverFailed')));
    } finally {
      imageBusy = false;
    }
  }

  async function mark(fn: () => Promise<unknown>, fallback: string) {
    try {
      await fn();
    } catch (err) {
      toast(markError(err, fallback), 'danger');
    }
  }

  let favoritePulse = $state(false);

  function toggleFavorite() {
    const current = page.localGame;
    if (!current) return;
    const next = !current.favorite;
    void mark(async () => {
      await setFavorite(current.id, next);
      favoritePulse = next;
    }, msg('games.errorFavoriteFailed'));
  }

  async function toggleRequiresSteam(on: boolean) {
    const current = page.localGame;
    if (!current || requiresSteamSaving) return;
    const previous = current.requiresSteam !== false;
    requiresSteamSaving = true;
    try {
      const updated = await setRequiresSteam(current.id, on);
      if (page.localGame?.id === current.id) requiresSteam = updated.requiresSteam !== false;
    } catch (err) {
      if (page.localGame?.id === current.id) requiresSteam = previous;
      toast(markError(err, msg('games.errorRequiresSteamFailed')), 'danger');
    } finally {
      if (page.localGame?.id === current.id) requiresSteamSaving = false;
    }
  }

  async function createDesktopShortcut() {
    if (!page.localGame) return;
    try {
      await createShortcut(page.localGame.id);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.detailShortcutCreateError')), 'danger');
    }
  }

  async function removeDesktopShortcut() {
    if (!page.localGame) return;
    try {
      await removeShortcut(page.localGame.id);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.detailShortcutRemoveError')), 'danger');
    }
  }

  let addingToLibrary = $state(false);

  async function addToLibrary() {
    if (!page.canonicalId || addingToLibrary) return;
    addingToLibrary = true;
    try {
      await addCatalogGame(page.canonicalId, page.title, page.coverSrc);
      toast(msg('games.detailAddedToLibraryToast'), 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('games.detailAddToLibraryError')), 'danger');
    } finally {
      addingToLibrary = false;
    }
  }

  return {
    get requiresSteam() { return requiresSteam; },
    set requiresSteam(on: boolean) { requiresSteam = on; },
    get requiresSteamSaving() { return requiresSteamSaving; },
    get favoritePulse() { return favoritePulse; },
    set favoritePulse(on: boolean) { favoritePulse = on; },
    get addingToLibrary() { return addingToLibrary; },
    get releaseGroups() { return releaseGroups; },
    get releasesLoading() { return releasesLoading; },
    get releasesFailed() { return releasesFailed; },
    get related() { return related; },
    get relatedLoading() { return relatedLoading; },
    get relatedFailed() { return relatedFailed; },
    get relatedLoaded() { return relatedLoaded; },
    get catalogGame() { return catalogGame; },
    get catalogLoading() { return catalogLoading; },
    get catalogFailed() { return catalogFailed; },
    get catalogRetrying() { return catalogRetrying; },
    get metaView() { return metaView; },
    set metaView(view: MetadataView | null) {
      metaView = view;
      if (view) metaReadFailed = false;
    },
    get metaReadFailed() { return metaReadFailed; },
    get metaRefreshing() { return metaRefreshing; },
    get metaSkipping() { return metaSkipping; },
    get metaSearchRan() { return metaSearchRan; },
    get metaRetrying() { return metaRetrying; },
    set metaRetrying(on: boolean) { metaRetrying = on; },
    get metaState() { return metaState; },
    get metaFailure() { return metaFailure; },
    get metaRetryable() { return metaRetryable; },
    get wineStatus() { return wineStatus; },
    get savePaths() { return savePaths; },
    get imageBusy() { return imageBusy; },
    loadReleases,
    loadRelated,
    retryReleases,
    retryRelated,
    retryCatalog,
    retryMetaRead,
    refreshMeta,
    skipMeta,
    chooseImage,
    findCover,
    toggleFavorite,
    toggleRequiresSteam,
    createDesktopShortcut,
    removeDesktopShortcut,
    addToLibrary,
  };
}
