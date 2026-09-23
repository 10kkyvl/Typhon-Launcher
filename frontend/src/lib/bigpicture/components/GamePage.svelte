<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { ArrowDown, ArrowLeft, ArrowUp, Check, ChevronLeft, ChevronRight, Download, Heart, Play, RotateCw, Square, X } from '@lucide/svelte';
  import Artwork from '../../components/Artwork.svelte';
  import { galleryShots, languageLabel, pickHero } from '../../game/view';
  import { GAME_STATUSES, statusLabel, type GameStatus } from '../../game/status';
  import { t, type MessageKey, type Params } from '../../i18n';
  import { installActive, installStatusLabels, installations } from '../../stores/install';
  import { downloads } from '../../stores/downloads';
  import { libraryGames, runningGames } from '../../stores/library';
  import { settings } from '../../stores/settings';
  import { addCatalogGame, setFavorite, setStatus, stopGame, type LibraryGame } from '../../services/library';
  import { cancelDownload, pauseDownload, resumeDownload, type Download as DownloadItem, type TorrentInfo } from '../../services/downloads';
  import { getCatalogGame, getReleasesForGame, getReleasesForTitle, type CatalogGame, type ReleaseGroup } from '../../services/sources';
  import { getMetadataView, ensureMetadataFresh, type MetadataView } from '../../services/metadata';
  import { appInfo, elevationSupported, type AppInfo } from '../../services/system';
  import { offerElevateAhead } from '../../services/install';
  import { selectFolder } from '../../services/settings';
  import { LatestRequestGate } from '../latestRequest';
  import { GameDownloadFlow } from '../gameDownload';
  import type { BigPictureCommand } from '../input';
  import { focusControl } from '../navigation';
  import type { RequestText } from '../contracts';
  import { bytesSize, progressPercent, relativeDate } from '../../utils/format';
  import { installErrorText } from '../../install/installErrors';
  import { sourceErrorText } from '../../sources/sourceErrors';
  import { errorCode, hasMessage } from '../../i18n';

  import { bigpictureCatalog } from '../../i18n/catalog/en/bigpictureCatalog';

  let { id, onback, ondownloads, onlaunch, requestText }: {
    id: string;
    onback: () => void;
    ondownloads: () => void;
    onlaunch: (id: string) => Promise<boolean>;
    requestText: RequestText;
  } = $props();

  type CatalogMessage = keyof typeof bigpictureCatalog;
  const bp = (key: CatalogMessage, params?: Params) => $t(key as MessageKey, params);
  type Dialog = 'download' | 'status' | 'stop' | 'cancel-download' | 'screenshot' | 'description' | null;
  type DownloadStep = 'preparing' | 'fetching' | 'files' | 'prepare-error' | 'fetch-error';

  const gameRequests = new LatestRequestGate();
  const releaseRequests = new LatestRequestGate();
  const downloadFlow = new GameDownloadFlow();

  let catalogGame = $state<CatalogGame | null>(null);
  let metadata = $state<MetadataView | null>(null);
  let detailsLoading = $state(false);
  let detailsFailed = $state(false);
  let releaseGroups = $state<ReleaseGroup[]>([]);
  let releasesLoading = $state(false);
  let releasesFailed = $state(false);
  let activeTab = $state<'overview' | 'releases'>('overview');
  let pageError = $state('');
  let dialog = $state<Dialog>(null);
  let dialogRoot: HTMLElement | undefined = $state();
  let dialogReturnFocus = '';
  let cancelDownloadId = '';
  let pendingReleaseId = $state('');
  let downloadStep = $state<DownloadStep>('preparing');
  let dialogError = $state('');
  let torrent = $state<TorrentInfo | null>(null);
  let selectedFiles = $state<boolean[]>([]);
  let destination = $state('');
  let autoInstall = $state(false);
  let elevateAhead = $state(false);
  let platform = $state<AppInfo | null>(null);
  let elevationCheckFailed = $state(false);
  let startingDownload = $state(false);
  let changingFavorite = $state(false);
  let changingStatus = $state(false);
  let launching = $state(false);
  let stopping = $state(false);
  let changingDownloadState = $state(false);
  let cancelingDownload = $state(false);
  let addingToLibrary = $state(false);
  let screenshotIndex = $state(0);
  let descriptionReader: HTMLDivElement | undefined = $state();
  let detailLoadId = '';
  let releaseLoadKey = '';
  let eventOff: (() => void) | undefined;

  const localGame = $derived(
    $libraryGames.find((game) => game.id === id) ??
      $libraryGames.find((game) => game.canonicalGameId === id || (game.canonicalGameId && catalogGame?.aliasIds?.includes(game.canonicalGameId))),
  );
  const title = $derived(localGame?.title || metadata?.game.title || catalogGame?.title || bp('bp.game.unknownTitle'));
  const canonicalId = $derived(localGame?.canonicalGameId || catalogGame?.id || id);
  const releaseCanonicalId = $derived(localGame?.canonicalGameId || catalogGame?.id || (localGame ? '' : id));
  const installed = $derived(Boolean(localGame && !localGame.uninstalled));
  const running = $derived(Boolean(localGame && $runningGames.has(localGame.id)));
  const cover = $derived(metadata?.cover || catalogGame?.coverUrl || localGame?.cover || '');
  const shots = $derived(metadata?.screenshots ?? []);
  const hero = $derived(pickHero(metadata?.hero || '', shots) || cover);
  const displayShots = $derived(galleryShots(shots, hero));
  const summary = $derived(metadata?.game.summary || catalogGame?.summary || '');
  const year = $derived(metadata?.game.releaseYear || catalogGame?.releaseYear || (metadata?.game.releaseDate ? new Date(metadata.game.releaseDate).getFullYear() : 0));
  const gameMeta = $derived([year || '', metadata?.game.developer || catalogGame?.developer, metadata?.game.publisher || catalogGame?.publisher].filter(Boolean).join(' · '));
  const genres = $derived([...new Set([...(metadata?.game.genres ?? []), ...(catalogGame?.genres ?? [])])]);
  const availableReleases = $derived(releaseGroups.filter((group) => group.release.availability !== 'removed'));
  const downloadRows = $derived.by(() => $downloads
    .filter((item) => ownsDownload(item))
    .toSorted((a, b) => Date.parse(b.addedAt) - Date.parse(a.addedAt)));
  const currentDownload = $derived(downloadRows[0]);
  const downloadIds = $derived(new Set(downloadRows.map((item) => item.id)));
  const currentInstall = $derived.by(() => $installations
    .filter((item) => downloadIds.has(item.downloadId) || Boolean(localGame && item.gameId === localGame.id))
    .toSorted((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt))[0]);
  const selectedPaths = $derived((torrent?.files ?? []).filter((_, index) => selectedFiles[index]).map((file) => file.path));
  const selectedCount = $derived(selectedFiles.filter(Boolean).length);
  const selectedBytes = $derived((torrent?.files ?? []).reduce((total, file, index) => total + (selectedFiles[index] ? file.size : 0), 0));
  const canElevate = $derived(offerElevateAhead({ elevationSupported: Boolean(platform && elevationSupported(platform)), autoInstall, paths: selectedPaths }));
  const canStartDownload = $derived(selectedCount > 0 && Boolean(destination.trim()) && Boolean($settings?.libraryPath) && !startingDownload);

  $effect(() => {
    const targetId = id;
    const knownGame = localGame;
    untrack(() => {
      void loadDetails(targetId, knownGame);
    });
  });

  $effect(() => {
    const key = `${id}|${releaseCanonicalId}|${localGame?.title ?? catalogGame?.title ?? ''}`;
    const gameId = releaseCanonicalId;
    const gameTitle = localGame?.title ?? catalogGame?.title ?? '';
    untrack(() => {
      if (key === releaseLoadKey) return;
      releaseLoadKey = key;
      void loadReleases(gameId, gameTitle);
    });
  });

  onMount(() => {
    if (!Events?.On) return;
    eventOff = Events.On('metadata:updated', (event) => {
      const update = event.data as MetadataView;
      if (update?.game?.id === canonicalId) metadata = update;
    });
  });

  onDestroy(() => {
    gameRequests.invalidate();
    releaseRequests.invalidate();
    void downloadFlow.cancel();
    eventOff?.();
  });

  async function loadDetails(targetId: string, game: LibraryGame | undefined) {
    if (detailLoadId === targetId) return;
    detailLoadId = targetId;
    const gameId = game?.canonicalGameId || targetId;
    const ticket = gameRequests.begin();
    detailsLoading = true;
    detailsFailed = false;
    catalogGame = null;
    metadata = null;
    activeTab = 'overview';
    const result = await gameRequests.settle(ticket, Promise.all([getCatalogGame(gameId), getMetadataView(gameId)]));
    if (result.kind === 'stale') return;
    if (result.kind === 'error') {
      detailsFailed = true;
    } else {
      [catalogGame, metadata] = result.value;
      if (!catalogGame && !game) detailsFailed = true;
      void ensureMetadataFresh(gameId).catch(() => undefined);
    }
    if (gameRequests.isCurrent(ticket)) detailsLoading = false;
  }

  async function loadReleases(gameId: string, gameTitle: string) {
    const ticket = releaseRequests.begin();
    releasesLoading = true;
    releasesFailed = false;
    const request = gameId ? getReleasesForGame(gameId) : gameTitle ? getReleasesForTitle(gameTitle) : Promise.resolve([]);
    const result = await releaseRequests.settle(ticket, request);
    if (result.kind === 'stale') return;
    if (result.kind === 'error') {
      releasesFailed = true;
      releaseGroups = [];
    } else {
      releasesFailed = false;
      releaseGroups = result.value;
    }
    if (releaseRequests.isCurrent(ticket)) releasesLoading = false;
  }

  function ownsDownload(item: DownloadItem): boolean {
    const origin = item.origin ?? {};
    const identifiers = [id, canonicalId, catalogGame?.id, localGame?.canonicalGameId].filter(Boolean);
    return Boolean((origin.gameId && identifiers.includes(origin.gameId)) || (localGame && origin.libraryId === localGame.id));
  }

  function focusKey(key: string) {
    const nodes = [...document.querySelectorAll<HTMLButtonElement>('[data-bp-focus]')];
    const found = nodes.find((node) => node.dataset.bpFocus === key && node.getClientRects().length > 0 && !node.disabled);
    if (found) focusControl(found);
  }

  async function focusDialogDefault() {
    await tick();
    if (!dialog || !dialogRoot) return;
    const target = dialogRoot.querySelector<HTMLButtonElement>('[data-bp-default]') ?? dialogRoot.querySelector<HTMLButtonElement>('[data-bp-focus]:not(:disabled)');
    focusControl(target ?? undefined);
  }

  function rememberFocus() {
    const active = document.activeElement as HTMLElement | null;
    dialogReturnFocus = active?.dataset?.bpFocus ?? '';
  }

  function showDialog(next: Exclude<Dialog, null>) {
    if (!dialog) rememberFocus();
    dialogError = '';
    dialog = next;
    void focusDialogDefault();
  }

  function openScreenshot(index: number) {
    screenshotIndex = index;
    showDialog('screenshot');
  }

  function openDescription() {
    showDialog('description');
  }

  function scrollDescription(direction: 'up' | 'down') {
    const reader = descriptionReader;
    if (!reader) return;
    const reducedMotion = document.documentElement.classList.contains('no-anim') ||
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true;
    const distance = Math.max(220, Math.round(reader.clientHeight * 0.72));
    reader.scrollBy({ top: direction === 'down' ? distance : -distance, behavior: reducedMotion ? 'instant' : 'smooth' });
  }

  async function closeDialog() {
    if (startingDownload) return;
    const closing = dialog;
    dialog = null;
    if (closing === 'download') {
      void downloadFlow.cancel();
      pendingReleaseId = '';
      torrent = null;
      selectedFiles = [];
      dialogError = '';
    }
    await tick();
    if (dialogReturnFocus) focusKey(dialogReturnFocus);
    dialogReturnFocus = '';
  }

  export function handleCommand(command: BigPictureCommand): boolean {
    if (command === 'back' && dialog) {
      void closeDialog();
      return true;
    }
    if (dialog === 'description' && (command === 'up' || command === 'down')) {
      scrollDescription(command);
      return true;
    }
    if (dialog === 'screenshot' && (command === 'previous' || command === 'next')) {
      if (displayShots.length) screenshotIndex = (screenshotIndex + (command === 'next' ? 1 : displayShots.length - 1)) % displayShots.length;
      return true;
    }
    if (dialog) return false;
    if (command === 'previous' || command === 'next') {
      activeTab = command === 'previous' ? 'overview' : 'releases';
      return true;
    }
    return false;
  }

  async function addToLibrary() {
    if (localGame || !canonicalId || addingToLibrary) return;
    addingToLibrary = true;
    try {
      await addCatalogGame(canonicalId, title, cover);
    } catch (error) {
      pageError = mappedError(error, bp('bp.game.installAddError'));
    } finally {
      addingToLibrary = false;
    }
  }

  function mappedError(error: unknown, fallback: string) {
    const code = errorCode(error);
    return hasMessage(code) ? $t(code) : fallback;
  }

  async function toggleFavorite() {
    if (!localGame || changingFavorite) return;
    changingFavorite = true;
    pageError = '';
    try {
      await setFavorite(localGame.id, !localGame.favorite);
    } catch (error) {
      pageError = mappedError(error, bp('bp.game.favoriteError'));
    } finally {
      changingFavorite = false;
    }
  }

  async function updateStatus(value: GameStatus) {
    if (!localGame || changingStatus) return;
    changingStatus = true;
    try {
      await setStatus(localGame.id, value);
      await closeDialog();
    } catch (error) {
      dialogError = mappedError(error, bp('bp.game.statusError'));
    } finally {
      changingStatus = false;
    }
  }

  async function launch() {
    if (!localGame || !installed || running || launching) return;
    launching = true;
    pageError = '';
    try {
      await onlaunch(localGame.id);
    } catch (error) {
      pageError = mappedError(error, bp('bp.game.launchFailed'));
    } finally {
      launching = false;
    }
  }

  async function stop() {
    if (!localGame || !running || stopping) return;
    stopping = true;
    pageError = '';
    try {
      await stopGame(localGame.id);
      await closeDialog();
    } catch (error) {
      dialogError = mappedError(error, bp('bp.game.stopFailed'));
    } finally {
      stopping = false;
    }
  }

  function openReleases() {
    activeTab = 'releases';
  }

  async function beginDownload(releaseId: string) {
    pendingReleaseId = releaseId;
    dialogError = '';
    downloadStep = 'preparing';
    torrent = null;
    selectedFiles = [];
    const currentSettings = $settings;
    destination = currentSettings?.downloadsPath ?? '';
    autoInstall = currentSettings?.autoInstall ?? false;
    elevateAhead = currentSettings?.elevateAhead ?? false;
    platform = null;
    elevationCheckFailed = false;
    showDialog('download');
    void loadPlatform();
    const result = await downloadFlow.prepare(releaseId, (phase) => { downloadStep = phase; });
    applyDownloadResult(result);
  }

  async function retryPrepare() {
    const result = await downloadFlow.retry((phase) => { downloadStep = phase; });
    applyDownloadResult(result);
  }

  async function loadPlatform() {
    try {
      platform = await appInfo();
    } catch {
      platform = null;
      elevationCheckFailed = true;
    }
  }

  function applyDownloadResult(result: Awaited<ReturnType<GameDownloadFlow['prepare']>>) {
    if (result.kind === 'stale' || dialog !== 'download') return;
    if (result.kind === 'error') {
      downloadStep = result.phase;
      dialogError = result.phase === 'prepare-error'
        ? sourceErrorText(result.error, bp('bp.game.prepareFailed'))
        : installErrorText(result.error, bp('bp.game.downloadFetchFailed'));
      return;
    }
    torrent = result.torrent;
    selectedFiles = result.torrent.files.map(() => true);
    downloadStep = 'files';
    dialogError = '';
  }

  async function retryMetadata() {
    const result = await downloadFlow.retry((phase) => { downloadStep = phase; });
    applyDownloadResult(result);
  }

  async function changeDestination() {
    const value = await requestText({
      title: bp('bp.game.downloadDestinationTitle'),
      initialValue: destination,
      maxLength: 2048,
    });
    if (value === null) return;
    destination = value.trim();
  }

  async function browseDestination() {
    try {
      const path = await selectFolder(bp('bp.game.downloadDestinationTitle'));
      if (path) destination = path;
    } catch {
      dialogError = bp('bp.game.errorFallback');
    }
  }

  function toggleAllFiles() {
    if (!torrent) return;
    const select = selectedCount !== torrent.files.length;
    selectedFiles = torrent.files.map(() => select);
  }

  async function startDownload() {
    if (!torrent || !canStartDownload) return;
    const indices = selectedFiles.map((selected, index) => selected ? index : -1).filter((index) => index >= 0);
    startingDownload = true;
    dialogError = '';
    try {
      await downloadFlow.start(destination.trim(), indices, {
        autoInstall,
        elevateAhead: autoInstall && canElevate && elevateAhead,
      });
      startingDownload = false;
      await closeDialog();
    } catch (error) {
      dialogError = installErrorText(error, bp('bp.game.downloadStartFailed'));
    } finally {
      startingDownload = false;
    }
  }

  async function requestCancelDownload() {
    if (!currentDownload || !['queued', 'metadata', 'downloading', 'paused', 'verifying'].includes(currentDownload.status)) return;
    cancelDownloadId = currentDownload.id;
    showDialog('cancel-download');
  }

  async function confirmCancelDownload() {
    if (!cancelDownloadId || cancelingDownload) return;
    cancelingDownload = true;
    dialogError = '';
    try {
      await cancelDownload(cancelDownloadId);
      cancelDownloadId = '';
      await closeDialog();
    } catch (error) {
      dialogError = installErrorText(error, bp('bp.game.cancelDownloadFailed'));
    } finally {
      cancelingDownload = false;
    }
  }

  async function setDownloadPaused(paused: boolean) {
    if (!currentDownload || changingDownloadState) return;
    const downloadId = currentDownload.id;
    changingDownloadState = true;
    try {
      if (paused) await resumeDownload(downloadId);
      else await pauseDownload(downloadId);
      pageError = '';
    } catch (error) {
      pageError = installErrorText(error, bp('bp.game.errorFallback'));
    } finally {
      changingDownloadState = false;
    }
  }

  function downloadStatus(item: DownloadItem) {
    switch (item.status) {
      case 'queued': return bp('bp.game.downloadQueued');
      case 'metadata': return bp('bp.game.downloadMetadata');
      case 'paused': return bp('bp.game.downloadPaused');
      case 'verifying': return bp('bp.game.downloadVerifying');
      case 'completed': return bp('bp.game.downloadCompleted');
      case 'failed': return bp('bp.game.downloadFailed');
      case 'downloading': return bp('bp.game.downloadProgress', { percent: progressPercent(item.progress) });
    }
  }

  function installStatusText() {
    if (!currentInstall) return '';
    if (currentInstall.status === 'completed') return bp('bp.game.installComplete');
    if (currentInstall.status === 'failed' || currentInstall.status === 'interrupted' || currentInstall.status === 'cancelled') return bp('bp.game.installFailed');
    return installStatusLabels(currentInstall.status);
  }

  function formattedSize(value: number) {
    return bytesSize(value);
  }
</script>

<div class="bp-game">
  <div class="game-content" inert={Boolean(dialog)}>
    <header class="page-head">
      <button class="large-button secondary back-button" data-bp-focus="game:back" onclick={onback}><ArrowLeft size="2rem" />{bp('bp.game.back')}</button>
      <div class="head-tabs" role="tablist" aria-label={title}>
        <button class="tab" class:selected={activeTab === 'overview'} role="tab" aria-selected={activeTab === 'overview'} data-bp-focus="game:tab:overview" onclick={() => activeTab = 'overview'}>{bp('bp.game.tabOverview')}</button>
        <button class="tab" class:selected={activeTab === 'releases'} role="tab" aria-selected={activeTab === 'releases'} data-bp-focus="game:tab:releases" onclick={openReleases}>{bp('bp.game.tabReleases')}<span>{availableReleases.length}</span></button>
      </div>
    </header>

    {#if detailsLoading && !catalogGame && !localGame}
      <div class="loading-state" role="status"><span class="spinner"></span><span>{bp('bp.game.loading')}</span></div>
    {:else if detailsFailed && !catalogGame && !localGame}
      <section class="bp-empty" role="alert">
        <h1>{bp('bp.game.loadFailed')}</h1>
        <button class="large-button primary" data-bp-focus="game:retry-details" data-bp-default onclick={() => { detailLoadId = ''; void loadDetails(id, localGame); }}><RotateCw size="1.8rem" />{bp('bp.game.retry')}</button>
      </section>
    {:else}
      <section class="hero" style:background-image={`linear-gradient(90deg, var(--surface-0) 0%, color-mix(in srgb, var(--surface-0) 85%, transparent) 55%, color-mix(in srgb, var(--surface-0) 40%, transparent) 100%), linear-gradient(0deg, var(--surface-0), transparent 70%), url("${hero}")`}>
        <div class="hero-cover"><Artwork src={cover} alt={title} label={title} ratio="3 / 4" radius="1.4rem" /></div>
        <div class="hero-info">
          <span class="eyebrow">{bp('bp.game.eyebrow')}</span>
          <h1>{title}</h1>
          {#if gameMeta}<p class="meta-line">{gameMeta}</p>{/if}
          {#if genres.length}<div class="genre-list">{#each genres.slice(0, 5) as genre (genre)}<span>{genre}</span>{/each}</div>{/if}
          <div class="badges">
            {#if running}<span class="badge running"><span class="dot"></span>{bp('bp.game.running')}</span>
            {:else if installed}<span class="badge installed">{bp('bp.game.installed')}</span>
            {:else if localGame}<span class="badge">{bp('bp.game.notInstalled')}</span>{/if}
            {#if localGame?.status}<span class="badge status">{statusLabel(localGame.status)}</span>{/if}
          </div>
          <div class="hero-actions">
            {#if installed && localGame}
              {#if running}
                <button class="large-button danger" data-bp-focus="game:play-toggle" data-bp-default onclick={() => { pageError = ''; showDialog('stop'); }}><Square size="1.8rem" fill="currentColor" />{bp('bp.game.stop')}</button>
              {:else}
                <button class="large-button primary" data-bp-focus="game:play-toggle" data-bp-default disabled={launching} onclick={() => void launch()}><Play size="1.9rem" fill="currentColor" />{bp('bp.game.launch')}</button>
              {/if}
            {:else if currentInstall && installActive(currentInstall.status)}
              <button class="large-button primary" data-bp-focus="game:hero:continue-install" data-bp-default onclick={ondownloads}><Download size="1.9rem" />{bp('bp.game.installContinue')}</button>
            {:else if currentDownload?.status === 'completed'}
              <button class="large-button primary" data-bp-focus="game:hero:continue-install" data-bp-default onclick={ondownloads}><Download size="1.9rem" />{bp('bp.game.installContinue')}</button>
            {:else if currentDownload}
              <button class="large-button primary" data-bp-focus="game:download-status" data-bp-default onclick={ondownloads}><Download size="1.9rem" />{bp('bp.game.downloads')}</button>
            {:else}
              <button class="large-button primary" data-bp-focus="game:see-releases" data-bp-default onclick={openReleases}><Download size="1.9rem" />{bp('bp.game.tabReleases')}</button>
            {/if}
            {#if !localGame && canonicalId}
              <button class="large-button secondary" data-bp-focus="game:add-library" onclick={() => void addToLibrary()}><Check size="1.8rem" />{bp('bp.game.addToLibrary')}</button>
            {/if}
            {#if localGame}
              <button class="large-button secondary" data-bp-focus="game:favorite" aria-pressed={Boolean(localGame.favorite)} disabled={changingFavorite} onclick={() => void toggleFavorite()}><Heart size="1.8rem" fill={localGame.favorite ? 'currentColor' : 'none'} />{localGame.favorite ? bp('bp.game.favoriteRemove') : bp('bp.game.favoriteAdd')}</button>
              <button class="large-button secondary" data-bp-focus="game:status" onclick={() => { dialogError = ''; showDialog('status'); }}>{bp('bp.game.status')}: {statusLabel(localGame.status)}</button>
            {/if}
          </div>
        </div>
      </section>

      {#if pageError}<div class="page-error" role="alert">{pageError}</div>{/if}

      {#if currentDownload}
        <section class="state-card" aria-label={bp('bp.game.downloadStatus')}>
          <div class="state-heading">
            <div><span class="section-kicker">{bp('bp.game.downloadStatus')}</span><h2>{currentDownload.name}</h2></div>
            <span class="state-label">{downloadStatus(currentDownload)}</span>
          </div>
          <div class="progress-track" role="progressbar" aria-valuenow={progressPercent(currentDownload.progress)} aria-valuemin="0" aria-valuemax="100"><span style:width={`${progressPercent(currentDownload.progress)}%`}></span></div>
          <div class="progress-foot"><span>{formattedSize(currentDownload.downloaded)} / {formattedSize(currentDownload.total)}</span><span>{progressPercent(currentDownload.progress)}%</span></div>
          <div class="state-actions">
            {#if ['downloading', 'metadata', 'verifying'].includes(currentDownload.status)}
              <button class="large-button secondary" data-bp-focus="game:download-toggle" disabled={changingDownloadState} onclick={() => void setDownloadPaused(false)}>{bp('bp.game.downloadPause')}</button>
            {:else if ['queued', 'paused', 'failed'].includes(currentDownload.status)}
              <button class="large-button secondary" data-bp-focus="game:download-toggle" disabled={changingDownloadState} onclick={() => void setDownloadPaused(true)}>{currentDownload.status === 'failed' ? bp('bp.game.downloadRetry') : bp('bp.game.downloadResume')}</button>
            {/if}
            {#if ['queued', 'metadata', 'downloading', 'paused', 'verifying'].includes(currentDownload.status)}
              <button class="large-button secondary" data-bp-focus="game:download-cancel" onclick={() => void requestCancelDownload()}>{bp('bp.game.downloadCancel')}</button>
            {:else}
              <button class="large-button primary" data-bp-focus="game:open-downloads" onclick={ondownloads}>{bp('bp.game.downloads')}</button>
            {/if}
          </div>
        </section>
      {/if}

      {#if currentInstall}
        <section class="state-card install-card" aria-label={installStatusText()}>
          <div class="state-heading"><div><span class="section-kicker">{bp('bp.game.installation')}</span><h2>{currentInstall.name}</h2></div><span class="state-label">{installStatusText()}</span></div>
          {#if installActive(currentInstall.status)}
            <div class="progress-track" role="progressbar" aria-valuenow={progressPercent(currentInstall.progress)} aria-valuemin="0" aria-valuemax="100"><span style:width={`${progressPercent(currentInstall.progress)}%`}></span></div>
            <div class="progress-foot"><span>{currentInstall.currentFile || installStatusLabels(currentInstall.status)}</span><span>{progressPercent(currentInstall.progress)}%</span></div>
          {:else if currentInstall.status === 'failed' || currentInstall.status === 'interrupted'}
            <p class="install-error">{installErrorText(currentInstall.error, bp('bp.game.installFailed'))}</p>
          {/if}
          {#if currentInstall.status !== 'completed'}
            <div class="state-actions"><button class="large-button primary" data-bp-focus="game:install:continue" onclick={ondownloads}>{bp('bp.game.installContinue')}</button></div>
          {/if}
        </section>
      {/if}

      {#if activeTab === 'overview'}
        <section class="detail-layout">
          <article class="description-card">
            <h2>{bp('bp.game.description')}</h2>
            {#if summary}
              <p class="summary summary-preview">{summary}</p>
              <button class="large-button secondary description-open" data-bp-focus="game:description:open" onclick={openDescription}>{bp('bp.game.readDescription')}<ChevronRight size="1.8rem" /></button>
            {:else}<p class="muted">{bp('bp.game.noDescription')}</p>{/if}
          </article>
          <article class="screenshots-card">
            <h2>{bp('bp.game.screenshots')}</h2>
            {#if displayShots.length}
              <div class="screenshots">
                {#each displayShots as shot, index (shot.id)}
                  <button class="shot" data-bp-focus={`game:screenshot:${encodeURIComponent(shot.id)}`} aria-label={bp('bp.game.screenshotOpen', { number: index + 1 })} onclick={() => openScreenshot(index)}><img src={shot.url || shot.path} alt="" loading="lazy" /></button>
                {/each}
              </div>
            {:else}<p class="muted">{bp('bp.game.noScreenshots')}</p>{/if}
          </article>
        </section>
      {:else}
        <section class="release-section" aria-label={bp('bp.game.releases')}>
          <div class="section-title"><div><span class="section-kicker">{bp('bp.game.eyebrow')}</span><h2>{bp('bp.game.releases')}</h2></div>
            {#if releasesLoading}<span class="loading-inline">{bp('bp.game.releaseLoading')}</span>{/if}
          </div>
          {#if releasesFailed}
            <div class="notice error" role="alert"><span>{bp('bp.game.releaseFailed')}</span><button class="large-button secondary" data-bp-focus="game:retry-releases" onclick={() => void loadReleases(releaseCanonicalId, localGame?.title ?? catalogGame?.title ?? '')}><RotateCw size="1.7rem" />{bp('bp.game.releaseRetry')}</button></div>
          {:else if releasesLoading && !releaseGroups.length}
            <div class="loading-state compact" role="status"><span class="spinner"></span><span>{bp('bp.game.releaseLoading')}</span></div>
          {:else if !availableReleases.length}
            <div class="bp-empty small"><h2>{bp('bp.game.releaseEmpty')}</h2></div>
          {:else}
            <div class="release-grid">
              {#each releaseGroups as group (group.release.id)}
                {@const release = group.release}
                {@const removed = release.availability === 'removed'}
                <button class="release-card" class:unavailable={removed} disabled={removed || Boolean(pendingReleaseId === release.id)} data-bp-focus={`game:release:${encodeURIComponent(release.id)}`} onclick={() => void beginDownload(release.id)}>
                  <span class="release-heading"><strong>{release.version || release.rawTitle}</strong>{#if release.new}<span class="new-badge">New</span>{/if}</span>
                  <span class="release-name">{release.rawTitle}</span>
                  {#if release.edition}<span class="release-edition">{release.edition}</span>{/if}
                  <span class="release-info"><span>{formattedSize(release.size)}</span>{#if release.languages?.length}<span>{languageLabel(release.languages)}</span>{/if}<span>{relativeDate(release.uploadedAt)}</span></span>
                  <span class="release-source">{group.sourceName}{#if release.repacker} · {release.repacker}{/if}{#if group.duplicates?.length}<span> · +{group.duplicates.length}</span>{/if}</span>
                  {#if removed}<span class="release-unavailable">{bp('bp.game.releaseRemoved')}</span>{:else}<span class="release-cta">{pendingReleaseId === release.id ? bp('bp.game.prepareRelease') : bp('bp.game.releaseDownload')}<ChevronRight size="1.8rem" /></span>{/if}
                </button>
              {/each}
            </div>
          {/if}
        </section>
      {/if}
    {/if}
  </div>

  {#if dialog}
    <div class="dialog-backdrop">
      <div class="bp-dialog" class:screenshot-dialog={dialog === 'screenshot'} class:description-dialog={dialog === 'description'} role="dialog" aria-modal="true" aria-labelledby="bp-game-dialog-title" tabindex="-1" bind:this={dialogRoot} data-bp-scope>
        {#if dialog === 'download'}
          <header class="dialog-head"><div><span class="section-kicker">{bp('bp.game.downloads')}</span><h2 id="bp-game-dialog-title">{bp('bp.game.downloadTitle')}</h2></div><button class="icon-button" aria-label={bp('bp.game.downloadClose')} data-bp-focus="game:dialog:download-close" onclick={() => void closeDialog()}><X size="2rem" /></button></header>
          {#if downloadStep === 'preparing'}
            <div class="loading-state compact" role="status"><span class="spinner"></span><span>{bp('bp.game.prepareRelease')}</span></div>
          {:else if downloadStep === 'prepare-error'}
            <div class="dialog-error" role="alert">{dialogError}</div>
            <div class="dialog-actions"><button class="large-button secondary" data-bp-focus="game:dialog:retry-prepare" data-bp-default onclick={() => void retryPrepare()}><RotateCw size="1.8rem" />{bp('bp.game.releaseRetry')}</button><button class="large-button secondary" data-bp-focus="game:dialog:close" onclick={() => void closeDialog()}>{bp('bp.game.cancel')}</button></div>
          {:else if downloadStep === 'fetching'}
            <div class="loading-state compact" role="status"><span class="spinner"></span><span>{bp('bp.game.downloadFetching')}</span></div>
          {:else if downloadStep === 'fetch-error'}
            <div class="dialog-error" role="alert">{dialogError}</div>
            <div class="dialog-actions"><button class="large-button secondary" data-bp-focus="game:dialog:retry-metadata" data-bp-default onclick={() => void retryMetadata()}><RotateCw size="1.8rem" />{bp('bp.game.releaseRetry')}</button><button class="large-button secondary" data-bp-focus="game:dialog:close" onclick={() => void closeDialog()}>{bp('bp.game.cancel')}</button></div>
          {:else if torrent}
            <div class="torrent-summary"><strong>{torrent.name}</strong><span>{formattedSize(torrent.totalBytes)}</span></div>
            <div class="destination-card"><div><span class="field-label">{bp('bp.game.downloadDestination')}</span><span class="path">{destination || '—'}</span></div><div class="destination-actions"><button class="large-button secondary" data-bp-focus="game:dialog:destination" onclick={() => void changeDestination()}>{bp('bp.game.changeDestination')}</button><button class="large-button secondary" data-bp-focus="game:dialog:browse-destination" onclick={() => void browseDestination()}>{bp('bp.game.browseFolder')}</button></div></div>
            <div class="file-heading"><span class="field-label">{bp('bp.game.selectFiles')}</span><button class="text-button" data-bp-focus="game:dialog:select-all" onclick={toggleAllFiles}>{selectedCount === torrent.files.length ? bp('bp.game.deselectAll') : bp('bp.game.selectAll')}</button></div>
            <div class="file-list" role="group" aria-label={bp('bp.game.selectFiles')}>
              {#each torrent.files as file, index (`${index}:${file.path}`)}
                <button class="file-row" role="checkbox" aria-checked={Boolean(selectedFiles[index])} data-bp-focus={`game:dialog:file:${index}`} onclick={() => selectedFiles[index] = !selectedFiles[index]}>
                  <span class="check-box" class:on={selectedFiles[index]}>{#if selectedFiles[index]}<Check size="1.6rem" />{/if}</span><span class="file-path">{file.path}</span><span class="file-size">{formattedSize(file.size)}</span>
                </button>
              {/each}
            </div>
            <div class="selected-summary">{bp('bp.game.selectedFiles', { count: selectedCount, size: formattedSize(selectedBytes) })}</div>
            {#if !$settings?.libraryPath}<div class="notice error" role="alert">{bp('bp.game.librarySetupRequired')}</div>{/if}
            <button class="option-row" role="checkbox" aria-checked={autoInstall} data-bp-focus="game:dialog:auto-install" onclick={() => autoInstall = !autoInstall}>
              <span class="check-box" class:on={autoInstall}>{#if autoInstall}<Check size="1.6rem" />{/if}</span><span><strong>{bp('bp.game.autoInstall')}</strong><small>{bp('bp.game.autoInstallHint')}</small></span>
            </button>
            {#if canElevate}<button class="option-row nested" role="checkbox" aria-checked={elevateAhead} data-bp-focus="game:dialog:elevate" onclick={() => elevateAhead = !elevateAhead}>
              <span class="check-box" class:on={elevateAhead}>{#if elevateAhead}<Check size="1.6rem" />{/if}</span><span><strong>{bp('bp.game.elevateAhead')}</strong><small>{bp('bp.game.elevateAheadHint')}</small></span>
            </button>{/if}
            {#if elevationCheckFailed}<p class="muted">{bp('bp.game.elevationFailed')}</p>{/if}
            {#if dialogError}<div class="dialog-error" role="alert">{dialogError}</div>{/if}
            {#if selectedCount === 0}<p class="dialog-error" role="status">{bp('bp.game.noFilesSelected')}</p>{/if}
            <div class="dialog-actions"><button class="large-button secondary" data-bp-focus="game:dialog:cancel" data-bp-default onclick={() => void closeDialog()}>{bp('bp.game.cancel')}</button><button class="large-button primary" data-bp-focus="game:dialog:start" disabled={!canStartDownload} onclick={() => void startDownload()}>{startingDownload ? bp('bp.game.downloadStarting') : bp('bp.game.downloadStart')}</button></div>
          {/if}
        {:else if dialog === 'screenshot'}
          {@const shot = displayShots.length ? displayShots[screenshotIndex % displayShots.length] : undefined}
          <header class="dialog-head"><div><span class="section-kicker">{bp('bp.game.screenshots')}</span><h2 id="bp-game-dialog-title">{title}</h2></div><button class="icon-button" aria-label={bp('bp.game.screenshotClose')} data-bp-focus="game:dialog:screenshot-close" data-bp-default onclick={() => void closeDialog()}><X size="2rem" /></button></header>
          {#if shot}
            <div class="screenshot-viewer">
              <button class="large-button secondary screenshot-nav" aria-label={bp('bp.game.screenshotPrevious')} data-bp-focus="game:dialog:screenshot-previous" onclick={() => screenshotIndex = (screenshotIndex + displayShots.length - 1) % displayShots.length}><ChevronLeft size="2.5rem" /></button>
              <figure><img src={shot.url || shot.path} alt={`${title} screenshot`} /><figcaption>{bp('bp.game.screenshotCounter', { current: screenshotIndex + 1, total: displayShots.length })}</figcaption></figure>
              <button class="large-button secondary screenshot-nav" aria-label={bp('bp.game.screenshotNext')} data-bp-focus="game:dialog:screenshot-next" onclick={() => screenshotIndex = (screenshotIndex + 1) % displayShots.length}><ChevronRight size="2.5rem" /></button>
            </div>
            <p class="viewer-hint">{bp('bp.game.screenshotNavigationHint')}</p>
          {/if}
        {:else if dialog === 'description'}
          <header class="dialog-head"><div><span class="section-kicker">{bp('bp.game.description')}</span><h2 id="bp-game-dialog-title">{bp('bp.game.descriptionDialogTitle')}</h2></div></header>
          <div class="description-reader" bind:this={descriptionReader} role="region" aria-label={bp('bp.game.description')} aria-describedby="bp-game-description-text">
            <p id="bp-game-description-text">{summary}</p>
          </div>
          <div class="description-actions">
            <button class="large-button secondary" data-bp-focus="game:dialog:description-up" aria-label={bp('bp.game.descriptionScrollUp')} onclick={() => scrollDescription('up')}><ArrowUp size="1.8rem" />{bp('bp.game.descriptionScrollUp')}</button>
            <button class="large-button secondary" data-bp-focus="game:dialog:description-down" aria-label={bp('bp.game.descriptionScrollDown')} onclick={() => scrollDescription('down')}><ArrowDown size="1.8rem" />{bp('bp.game.descriptionScrollDown')}</button>
            <button class="large-button primary" data-bp-focus="game:dialog:description-close" data-bp-default onclick={() => void closeDialog()}><X size="1.8rem" />{bp('bp.game.descriptionClose')}</button>
          </div>
        {:else if dialog === 'status' && localGame}
          <header class="dialog-head"><div><span class="section-kicker">{bp('bp.game.status')}</span><h2 id="bp-game-dialog-title">{bp('bp.game.statusTitle')}</h2></div><button class="icon-button" aria-label={bp('bp.game.cancel')} data-bp-focus="game:dialog:status-close" data-bp-default onclick={() => void closeDialog()}><X size="2rem" /></button></header>
          <div class="status-options">
            <button class="status-option" aria-pressed={!localGame.status} data-bp-focus="game:dialog:status:none" onclick={() => void updateStatus('')}>{bp('bp.game.statusNone')}</button>
            {#each GAME_STATUSES as value (value)}<button class="status-option" aria-pressed={localGame.status === value} data-bp-focus={`game:dialog:status:${value}`} onclick={() => void updateStatus(value)}>{bp(`bp.game.status${value[0].toUpperCase()}${value.slice(1)}` as CatalogMessage)}</button>{/each}
          </div>
          {#if dialogError}<div class="dialog-error" role="alert">{dialogError}</div>{/if}
        {:else if dialog === 'stop'}
          <header class="dialog-head"><div><span class="section-kicker">{bp('bp.game.stop')}</span><h2 id="bp-game-dialog-title">{bp('bp.game.stopTitle')}</h2></div><button class="icon-button" aria-label={bp('bp.game.cancel')} data-bp-focus="game:dialog:stop-close" data-bp-default onclick={() => void closeDialog()}><X size="2rem" /></button></header>
          <p class="dialog-copy">{bp('bp.game.stopWarning')}</p>
          {#if dialogError}<div class="dialog-error" role="alert">{dialogError}</div>{/if}
          <div class="dialog-actions"><button class="large-button secondary" data-bp-focus="game:dialog:stop-cancel" onclick={() => void closeDialog()}>{bp('bp.game.cancel')}</button><button class="large-button danger" data-bp-focus="game:dialog:stop-confirm" onclick={() => void stop()} disabled={stopping}>{bp('bp.game.confirmStop')}</button></div>
        {:else if dialog === 'cancel-download'}
          <header class="dialog-head"><div><span class="section-kicker">{bp('bp.game.downloads')}</span><h2 id="bp-game-dialog-title">{bp('bp.game.cancelDownloadTitle')}</h2></div><button class="icon-button" aria-label={bp('bp.game.cancel')} data-bp-focus="game:dialog:cancel-download-close" data-bp-default onclick={() => void closeDialog()}><X size="2rem" /></button></header>
          <p class="dialog-copy">{bp('bp.game.cancelDownloadText')}</p>
          {#if dialogError}<div class="dialog-error" role="alert">{dialogError}</div>{/if}
          <div class="dialog-actions"><button class="large-button secondary" data-bp-focus="game:dialog:cancel-download-keep" onclick={() => void closeDialog()}>{bp('bp.game.cancel')}</button><button class="large-button danger" data-bp-focus="game:dialog:cancel-download-confirm" onclick={() => void confirmCancelDownload()} disabled={cancelingDownload}>{bp('bp.game.confirmCancelDownload')}</button></div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .bp-game { position: relative; min-height: 100%; color: var(--text); }
  .game-content { max-width: 160rem; margin: 0 auto; padding: 2rem clamp(2rem, 4vw, 6rem) 5rem; }
  .page-head { display: flex; align-items: center; gap: 2rem; margin-bottom: 2rem; }
  .head-tabs { display: flex; align-items: center; gap: .8rem; }
  .large-button { min-height: 5.6rem; display: inline-flex; align-items: center; justify-content: center; gap: .9rem; padding: 0 1.8rem; border: 1px solid transparent; border-radius: 1.2rem; background: var(--surface-2); color: var(--text); font-size: 1.55rem; font-weight: 700; line-height: 1.2; text-align: center; cursor: pointer; transition: transform 120ms ease, border-color 120ms ease, background 120ms ease; }
  .large-button:hover:not(:disabled), .tab:hover, .status-option:hover, .release-card:hover:not(:disabled) { transform: translateY(-1px); }
  .large-button.primary { background: var(--accent); color: var(--accent-on, white); }
  .large-button.secondary { border-color: var(--border); background: color-mix(in srgb, var(--surface-2) 86%, transparent); }
  .large-button.danger { background: color-mix(in srgb, var(--danger) 84%, black); color: white; }
  .large-button:disabled { opacity: .5; cursor: default; }
  :global([data-bp-focus]:focus-visible) { outline: 3px solid var(--accent); outline-offset: 4px; }
  .back-button { min-height: 4.8rem; padding: 0 1.4rem; font-size: 1.35rem; }
  .tab { min-height: 4.8rem; display: inline-flex; align-items: center; gap: .8rem; padding: 0 1.5rem; border: 1px solid var(--border); border-radius: 999px; background: var(--surface-1); color: var(--text-2); font-size: 1.45rem; font-weight: 700; cursor: pointer; }
  .tab.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 18%, var(--surface-1)); color: var(--text); }
  .tab span { min-width: 2.3rem; padding: .2rem .5rem; border-radius: 999px; background: var(--surface-3); color: var(--text-3); font-size: 1.2rem; text-align: center; }
  .hero { position: relative; isolation: isolate; display: grid; grid-template-columns: minmax(18rem, 26rem) minmax(0, 1fr); gap: clamp(2.4rem, 4vw, 6rem); align-items: center; min-height: 46rem; padding: clamp(2.4rem, 4vw, 5rem); overflow: hidden; border: 1px solid var(--border); border-radius: 2rem; background-color: var(--surface-1); background-size: cover; background-position: center; }
  .hero-cover { width: 100%; max-width: 24rem; overflow: hidden; border-radius: 1.4rem; box-shadow: 0 2rem 5rem rgba(0,0,0,.38); }
  .hero-info { display: flex; min-width: 0; flex-direction: column; align-items: flex-start; gap: 1.4rem; }
  .eyebrow, .section-kicker { color: var(--accent); font-size: 1.25rem; font-weight: 800; letter-spacing: .15em; text-transform: uppercase; }
  h1 { max-width: 88rem; margin: 0; font-size: clamp(3.4rem, 5vw, 7rem); line-height: 1.02; letter-spacing: -.04em; text-wrap: balance; }
  .meta-line { margin: 0; color: var(--text-2); font-size: 1.65rem; }
  .genre-list, .badges { display: flex; flex-wrap: wrap; gap: .8rem; }
  .genre-list span, .badge { min-height: 3.2rem; display: inline-flex; align-items: center; gap: .7rem; padding: 0 1rem; border: 1px solid var(--border); border-radius: 999px; background: color-mix(in srgb, var(--surface-1) 75%, transparent); color: var(--text-2); font-size: 1.25rem; font-weight: 650; }
  .badge.installed { border-color: color-mix(in srgb, var(--success) 45%, var(--border)); color: var(--success); }
  .badge.running { border-color: color-mix(in srgb, var(--accent) 50%, var(--border)); color: var(--accent); }
  .badge.status { color: var(--text); }
  .dot { width: .8rem; height: .8rem; border-radius: 50%; background: currentColor; }
  .hero-actions { display: flex; flex-wrap: wrap; gap: 1rem; margin-top: 1rem; }
  .page-error, .dialog-error { padding: 1.2rem 1.5rem; border: 1px solid color-mix(in srgb, var(--danger) 45%, var(--border)); border-radius: 1rem; background: color-mix(in srgb, var(--danger) 10%, var(--surface-1)); color: var(--danger); font-size: 1.45rem; line-height: 1.4; }
  .page-error { margin-top: 1.5rem; }
  .state-card, .description-card, .screenshots-card, .release-section { margin-top: 2rem; padding: 2.2rem; border: 1px solid var(--border); border-radius: 1.6rem; background: var(--surface-1); }
  .state-heading, .section-title { display: flex; align-items: center; justify-content: space-between; gap: 2rem; }
  .state-heading h2, .section-title h2, .description-card h2, .screenshots-card h2 { margin: .5rem 0 0; font-size: 2.2rem; line-height: 1.2; }
  .state-label { color: var(--text-2); font-size: 1.45rem; text-align: right; }
  .progress-track { height: 1rem; margin-top: 2rem; overflow: hidden; border-radius: 999px; background: var(--surface-3); }
  .progress-track span { display: block; height: 100%; border-radius: inherit; background: var(--accent); transition: width 200ms ease; }
  .progress-foot { display: flex; justify-content: space-between; gap: 2rem; margin-top: .8rem; color: var(--text-3); font-size: 1.3rem; font-variant-numeric: tabular-nums; }
  .state-actions { display: flex; flex-wrap: wrap; gap: 1rem; margin-top: 1.5rem; }
  .install-card { border-color: color-mix(in srgb, var(--accent) 35%, var(--border)); }
  .install-error { color: var(--danger); font-size: 1.4rem; }
  .detail-layout { display: grid; grid-template-columns: minmax(0, 1.15fr) minmax(28rem, .85fr); gap: 2rem; margin-top: 2rem; }
  .description-card, .screenshots-card { min-width: 0; margin: 0; }
  .summary { margin: 1.4rem 0 0; color: var(--text-2); font-size: 1.55rem; line-height: 1.65; white-space: pre-line; }
  .summary-preview { display: -webkit-box; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 5; line-clamp: 5; }
  .description-open { margin-top: 1.5rem; }
  .muted { color: var(--text-3); font-size: 1.45rem; line-height: 1.5; }
  .screenshots { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1rem; margin-top: 1.4rem; }
  .shot { display: block; overflow: hidden; width: 100%; padding: 0; aspect-ratio: 16 / 9; border: 0; border-radius: 1rem; background: var(--surface-3); cursor: pointer; }
  .shot img { width: 100%; height: 100%; object-fit: cover; }
  .bp-dialog.screenshot-dialog { width: min(148rem, 100%); height: min(92vh, 100rem); max-height: 92vh; overflow: hidden; }
  .screenshot-viewer { display: grid; min-height: 0; flex: 1; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 1.5rem; }
  .screenshot-viewer figure { display: flex; min-width: 0; height: 100%; flex-direction: column; align-items: center; justify-content: center; gap: 1rem; margin: 0; }
  .screenshot-viewer img { display: block; max-width: 100%; max-height: 72vh; object-fit: contain; }
  .screenshot-viewer figcaption, .viewer-hint { color: var(--text-3); font-size: 1.3rem; }
  .viewer-hint { margin: 0; text-align: center; }
  .screenshot-nav { width: 5.8rem; min-height: 5.8rem; padding: 0; }
  .description-dialog { width: min(88rem, 100%); }
  .description-reader { min-height: 20rem; max-height: min(58vh, 66rem); overflow: auto; padding: 2rem; border: 1px solid var(--border); border-radius: 1.2rem; background: var(--surface-1); color: var(--text-2); font-size: 1.65rem; line-height: 1.7; }
  .description-reader p { margin: 0; white-space: pre-line; }
  .description-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: .8rem; }
  @media (prefers-reduced-motion: reduce) { .description-reader { scroll-behavior: auto; } }
  .section-title { margin-bottom: 1.5rem; }
  .loading-inline { color: var(--text-3); font-size: 1.4rem; }
  .release-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 1.2rem; }
  .release-card { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: .9rem; padding: 1.6rem; border: 1px solid var(--border); border-radius: 1.4rem; background: var(--surface-2); color: var(--text); text-align: left; cursor: pointer; transition: transform 120ms ease, border-color 120ms ease; }
  .release-card:hover:not(:disabled) { border-color: var(--accent); }
  .release-card:disabled { opacity: .6; cursor: default; }
  .release-card.unavailable { opacity: .4; }
  .release-heading { display: flex; width: 100%; align-items: center; justify-content: space-between; gap: 1rem; }
  .release-heading strong { min-width: 0; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; font-size: 1.75rem; }
  .new-badge { padding: .35rem .65rem; border-radius: 999px; background: color-mix(in srgb, var(--accent) 20%, var(--surface-1)); color: var(--accent); font-size: 1.1rem; font-weight: 800; }
  .release-name { display: -webkit-box; overflow: hidden; color: var(--text-2); font-size: 1.35rem; line-height: 1.4; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }
  .release-edition { color: var(--text-3); font-size: 1.25rem; }
  .release-info { display: flex; flex-wrap: wrap; gap: .7rem 1.2rem; color: var(--text-3); font-size: 1.2rem; }
  .release-source { max-width: 100%; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; color: var(--text-3); font-size: 1.2rem; }
  .release-cta { display: flex; width: 100%; align-items: center; justify-content: space-between; margin-top: .3rem; padding-top: 1rem; border-top: 1px solid var(--border); color: var(--accent); font-size: 1.45rem; font-weight: 750; }
  .release-unavailable { color: var(--danger); font-size: 1.3rem; }
  .notice { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: 1.4rem; border-radius: 1rem; background: var(--surface-2); color: var(--text-2); font-size: 1.4rem; }
  .notice.error { color: var(--danger); }
  .bp-empty { display: flex; min-height: 30rem; flex-direction: column; align-items: center; justify-content: center; gap: 1.5rem; padding: 3rem; border: 1px dashed var(--border); border-radius: 1.5rem; text-align: center; }
  .bp-empty h1, .bp-empty h2 { margin: 0; font-size: 2.5rem; }
  .bp-empty.small { min-height: 14rem; }
  .loading-state { display: flex; min-height: 28rem; flex-direction: column; align-items: center; justify-content: center; gap: 1.5rem; color: var(--text-2); font-size: 1.6rem; }
  .loading-state.compact { min-height: 14rem; flex-direction: row; justify-content: flex-start; }
  .spinner { width: 3rem; height: 3rem; flex: 0 0 auto; border: 3px solid color-mix(in srgb, var(--accent) 22%, transparent); border-top-color: var(--accent); border-radius: 50%; animation: spin 850ms linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .dialog-backdrop { position: fixed; z-index: 100; inset: 0; display: grid; place-items: center; padding: 2rem; background: rgba(0,0,0,.76); }
  .bp-dialog { display: flex; width: min(88rem, 100%); max-height: min(90vh, 100rem); flex-direction: column; gap: 1.5rem; overflow: auto; padding: 2.4rem; border: 1px solid var(--border); border-radius: 1.8rem; background: var(--surface-0); box-shadow: 0 3rem 8rem rgba(0,0,0,.5); }
  .dialog-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 2rem; }
  .dialog-head h2 { margin: .5rem 0 0; font-size: 2.5rem; }
  .icon-button { width: 4.8rem; height: 4.8rem; display: grid; flex: 0 0 auto; place-items: center; border: 1px solid var(--border); border-radius: 1rem; background: var(--surface-2); color: var(--text); cursor: pointer; }
  .torrent-summary { display: flex; align-items: center; justify-content: space-between; gap: 2rem; font-size: 1.45rem; }
  .torrent-summary strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .torrent-summary span { flex: 0 0 auto; color: var(--text-3); }
  .destination-card { display: flex; align-items: center; justify-content: space-between; gap: 1.4rem; padding: 1.3rem; border: 1px solid var(--border); border-radius: 1.1rem; background: var(--surface-1); }
  .destination-card > div { display: flex; min-width: 0; flex-direction: column; gap: .4rem; }
  .field-label { color: var(--text-3); font-size: 1.25rem; font-weight: 700; }
  .path { overflow: hidden; color: var(--text-2); font-size: 1.25rem; text-overflow: ellipsis; white-space: nowrap; }
  .destination-actions { display: flex; flex-wrap: wrap; gap: .7rem; }
  .destination-card .large-button { min-height: 4.5rem; flex: 0 0 auto; padding-inline: 1.2rem; font-size: 1.25rem; }
  .file-heading { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
  .text-button { min-height: 3.8rem; padding: 0 .8rem; background: none; color: var(--accent); font-size: 1.3rem; font-weight: 700; cursor: pointer; }
  .file-list { display: flex; max-height: 25rem; flex-direction: column; gap: .5rem; overflow: auto; padding: .5rem; border: 1px solid var(--border); border-radius: 1.1rem; background: var(--surface-1); }
  .file-row { display: flex; min-height: 4.8rem; align-items: center; gap: 1rem; padding: .5rem .8rem; border: 1px solid transparent; border-radius: .8rem; background: transparent; color: var(--text); text-align: left; cursor: pointer; }
  .file-row:hover { background: var(--surface-2); }
  .check-box { width: 2.4rem; height: 2.4rem; display: grid; flex: 0 0 auto; place-items: center; border: 1px solid var(--border); border-radius: .5rem; color: var(--accent-on, white); }
  .check-box.on { border-color: var(--accent); background: var(--accent); }
  .file-path { min-width: 0; flex: 1; overflow: hidden; font-size: 1.25rem; text-overflow: ellipsis; white-space: nowrap; }
  .file-size { flex: 0 0 auto; color: var(--text-3); font-size: 1.2rem; font-variant-numeric: tabular-nums; }
  .selected-summary { color: var(--text-3); font-size: 1.25rem; text-align: right; }
  .option-row { display: flex; align-items: center; gap: 1rem; padding: 1rem; border: 1px solid transparent; border-radius: 1rem; background: var(--surface-1); color: var(--text); text-align: left; cursor: pointer; }
  .option-row.nested { margin-left: 3.4rem; }
  .option-row > span:last-child { display: flex; flex-direction: column; gap: .35rem; }
  .option-row strong { font-size: 1.35rem; }
  .option-row small { color: var(--text-3); font-size: 1.15rem; line-height: 1.4; }
  .dialog-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 1rem; margin-top: .5rem; }
  .dialog-copy { margin: 0; color: var(--text-2); font-size: 1.6rem; line-height: 1.5; }
  .status-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .8rem; }
  .status-option { min-height: 5.4rem; padding: 1rem 1.3rem; border: 1px solid var(--border); border-radius: 1rem; background: var(--surface-1); color: var(--text); font-size: 1.5rem; font-weight: 650; cursor: pointer; }
  .status-option[aria-pressed="true"] { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 16%, var(--surface-1)); }
  @media (max-width: 900px) { .hero { grid-template-columns: 18rem minmax(0, 1fr); min-height: 38rem; } .release-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .detail-layout { grid-template-columns: 1fr; } }
  @media (max-width: 640px) { .game-content { padding-inline: 1.3rem; } .page-head { align-items: stretch; flex-direction: column; } .hero { grid-template-columns: minmax(0, 1fr); justify-items: center; padding: 2rem; text-align: center; } .hero-cover { max-width: 18rem; } .hero-info { align-items: center; } .hero-actions { justify-content: center; } .release-grid { grid-template-columns: 1fr; } .state-heading, .destination-card { align-items: flex-start; flex-direction: column; } .state-label { text-align: left; } .dialog-backdrop { padding: .8rem; } .bp-dialog { max-height: 96vh; padding: 1.5rem; } .screenshot-dialog { width: 100%; height: 96vh; max-height: 96vh; } .screenshot-viewer { grid-template-columns: auto minmax(0, 1fr) auto; gap: .5rem; } .screenshot-nav { width: 4.2rem; min-height: 4.2rem; } .screenshot-viewer img { max-height: 72vh; } .description-reader { min-height: 16rem; max-height: 60vh; padding: 1.5rem; } .description-actions .large-button { min-width: 0; flex: 1; padding-inline: .8rem; font-size: 1.3rem; } .dialog-actions .large-button { flex: 1; } }
</style>
