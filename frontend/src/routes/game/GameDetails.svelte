<script lang="ts">
  import {
    BookmarkPlus,
    ChevronRight,
    Download,
    EllipsisVertical,
    FolderOpen,
    Heart,
    Play,
    Square,
  } from '@lucide/svelte';
  import { onMount, untrack } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import InstallModal from '../../lib/components/InstallModal.svelte';
  import Lightbox from '../../lib/components/Lightbox.svelte';
  import MetadataMatchModal from '../../lib/components/MetadataMatchModal.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import ReleaseList from '../../lib/components/ReleaseList.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import UpdateCard from '../../lib/components/UpdateCard.svelte';
  import VerifyCard from '../../lib/components/VerifyCard.svelte';
  import {
    busyState,
    clean,
    facts,
    galleryShots,
    cancelFreesDisk,
    hubAction,
    isGameMissing,
    joinLimited,
    metaLine,
    metaStatus,
    orderPlatforms,
    pickHero,
    preferView,
    summaryView,
    tagList,
    type TerminalDownloadStatus,
  } from '../../lib/game/view';
  import {
    cancelDownload,
    deleteDownloadData,
    removeDownload,
    resumeDownload,
    type Download as DownloadItem,
    type DownloadOrigin,
    type DownloadStatus,
  } from '../../lib/services/downloads';
  import {
    addCatalogGame,
    createShortcut,
    markError,
    playGame,
    removeShortcut,
    setCompleted,
    setFavorite,
    stopGame,
  } from '../../lib/services/library';
  import {
    dismissMetadataMatch,
    ensureMetadataFresh,
    getMetadataView,
    refreshMetadata,
    type MetadataView,
  } from '../../lib/services/metadata';
  import { openFolder } from '../../lib/services/settings';
  import {
    getCatalogGame,
    getReleasesForGame,
    getReleasesForTitle,
    prepareReleaseDownload,
    type CatalogGame,
    type ReleaseGroup,
  } from '../../lib/services/sources';
  import { getVerifyState } from '../../lib/services/updates';
  import { downloads, statusLabels } from '../../lib/stores/downloads';
  import { installActive, installStatusLabels, installations } from '../../lib/stores/install';
  import { libraryGames, runningGames } from '../../lib/stores/library';
  import { metadataAvailable } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { stepLabels, updatesByGame, verifications } from '../../lib/stores/updates';
  import { bytesLabel, playtime, relativeDate, truncateMiddle } from '../../lib/utils/format';

  let { id }: { id: string } = $props();

  const localGame = $derived(
    $libraryGames.find((g) => g.id === id) ?? $libraryGames.find((g) => g.canonicalGameId === id),
  );
  const installed = $derived(Boolean(localGame) && !localGame?.uninstalled);
  const running = $derived(localGame ? $runningGames.has(localGame.id) : false);

  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('library');

  const update = $derived(localGame ? $updatesByGame.get(localGame.id) : undefined);
  const verifyState = $derived(localGame ? $verifications[localGame.id] : undefined);
  const updateAvailable = $derived(Boolean(update?.availability.available));
  const showUpdateCard = $derived(
    Boolean(
      update &&
        (update.availability.available ||
          update.canRollback ||
          update.state === 'updating' ||
          update.state === 'update_downloading' ||
          update.state === 'update_failed'),
    ),
  );

  let updateCard = $state<UpdateCard | undefined>(undefined);

  async function loadVerifyState(gameId: string) {
    const state = await getVerifyState(gameId);
    if (!state) return;
    verifications.update((map) => (map[gameId] ? map : { ...map, [gameId]: state }));
  }

  $effect(() => {
    const gameId = localGame?.id;
    if (!gameId) return;
    untrack(() => {
      loadVerifyState(gameId);
    });
  });

  let releaseGroups = $state<ReleaseGroup[]>([]);
  let releasesLoading = $state(false);
  let releaseToken = 0;

  let catalogGame = $state<CatalogGame | null>(null);
  let catalogLoading = $state(false);
  let catalogToken = 0;

  async function loadCatalogGame(gameId: string) {
    const current = ++catalogToken;
    catalogLoading = true;
    try {
      const found = await getCatalogGame(gameId);
      if (current !== catalogToken) return;
      catalogGame = found;
    } finally {
      if (current === catalogToken) catalogLoading = false;
    }
  }

  $effect(() => {
    const gameId = id;
    const known = Boolean(localGame);
    untrack(() => {
      if (known) {
        catalogToken++;
        catalogLoading = false;
        catalogGame = null;
        return;
      }
      loadCatalogGame(gameId);
    });
  });

  const canonicalId = $derived(localGame ? localGame.canonicalGameId : catalogGame?.id || id);
  const releaseTitle = $derived(localGame?.title ?? catalogGame?.title);
  const releaseKey = $derived(`${id}|${canonicalId ?? ''}|${releaseTitle ?? ''}`);

  const anyOwnDownload = $derived($downloads.find((item) => ownsDownload(item)));

  let metaView = $state<MetadataView | null>(null);
  let metaToken = 0;
  let metaRefreshing = $state(false);
  let metaSearching = $state(false);
  let metaSkipping = $state(false);
  let matchOpen = $state(false);
  let matchMode = $state<'find' | 'change'>('find');
  let summaryExpanded = $state(false);
  let lightboxOpen = $state(false);
  let lightboxIndex = $state(0);
  let pickerOpen = $state(false);
  let heroFailed = $state(false);

  async function loadMetaView(gameId: string) {
    const current = ++metaToken;
    const view = await getMetadataView(gameId);
    if (current !== metaToken) return;
    metaView = preferView(metaView, view);
    const started = await ensureMetadataFresh(gameId);
    if (current !== metaToken || !started) return;
    metaSearching = true;
  }

  $effect(() => {
    const metaGameId = canonicalId;
    untrack(() => {
      metaSearching = false;
      if (!metaGameId) {
        metaToken++;
        metaView = null;
        return;
      }
      loadMetaView(metaGameId);
    });
  });

  onMount(() => {
    return Events.On('metadata:updated', (event) => {
      const view = event.data as MetadataView;
      if (view.game?.id && view.game.id === canonicalId) {
        metaSearching = false;
        metaView = preferView(metaView, view);
      }
    });
  });

  const metaState = $derived(
    metaStatus({
      available: $metadataAvailable,
      busy: metaSearching || metaRefreshing || metaSkipping,
      match: metaView?.match,
      resolved: metaView?.resolved,
    }),
  );

  const info = $derived(metaView?.game ?? catalogGame ?? null);
  const title = $derived(
    clean(localGame?.title) || clean(catalogGame?.title) || clean(info?.title) || clean(anyOwnDownload?.name),
  );
  const screenshots = $derived(metaView?.screenshots ?? []);
  const heroSrc = $derived(pickHero(metaView?.hero ?? '', screenshots));
  const shots = $derived(galleryShots(screenshots, heroSrc));
  const coverSrc = $derived(clean(metaView?.cover) || clean(localGame?.cover));
  const showHero = $derived(Boolean(heroSrc) && !heroFailed);

  $effect(() => {
    heroSrc;
    untrack(() => {
      heroFailed = false;
      lightboxIndex = 0;
    });
  });

  const releaseDateLabel = $derived.by(() => {
    if (!info) return '';
    if (info.releaseDate) {
      const date = new Date(info.releaseDate);
      if (!Number.isNaN(date.getTime())) return date.toLocaleDateString('ru-RU');
    }
    return info.releaseYear ? String(info.releaseYear) : '';
  });

  const releaseYear = $derived.by(() => {
    if (!info) return '';
    if (info.releaseYear) return String(info.releaseYear);
    if (info.releaseDate) {
      const date = new Date(info.releaseDate);
      if (!Number.isNaN(date.getTime())) return String(date.getFullYear());
    }
    return '';
  });

  const platforms = $derived(orderPlatforms(info?.platforms));

  const metaParts = $derived(
    metaLine({
      year: releaseYear,
      developer: info?.developer,
      publisher: info?.publisher,
      genres: info?.genres,
      platforms,
    }),
  );

  const tags = $derived(tagList([info?.genres, info?.themes]));
  const summary = $derived(summaryView(info?.summary ?? '', summaryExpanded));

  const gameFacts = $derived(
    facts([
      { label: 'Дата выхода', value: releaseDateLabel },
      { label: 'Разработчик', value: info?.developer },
      { label: 'Издатель', value: info?.publisher },
      { label: 'Платформы', value: joinLimited(platforms, 4), full: platforms.join(', ') },
    ]),
  );

  const shotCols = $derived(
    shots.length >= 5 ? 4 : shots.length === 4 ? 2 : Math.max(1, shots.length),
  );

  const installFacts = $derived.by(() => {
    if (!localGame || !installed) return [];
    const exe = localGame.executable.split(/[\\/]/).pop() ?? '';
    return facts([
      { label: 'Версия', value: localGame.version },
      { label: 'Размер', value: localGame.sizeBytes > 0 ? bytesLabel(localGame.sizeBytes) : '' },
      { label: 'Исполняемый файл', value: exe, full: localGame.executable, mono: true },
      { label: 'Наиграно', value: playtime(localGame.playtimeSeconds) },
      { label: 'Последний запуск', value: relativeDate(localGame.lastPlayed) },
      { label: 'Добавлена', value: relativeDate(localGame.installedAt) },
    ]);
  });

  const activeDownloadStatuses: DownloadStatus[] = ['queued', 'metadata', 'downloading', 'paused', 'verifying'];

  function ownsDownload(item: DownloadItem) {
    const origin = item.origin ?? {};
    if (canonicalId && origin.gameId === canonicalId) return true;
    return Boolean(localGame && origin.libraryId === localGame.id);
  }

  const ownDownload = $derived(
    $downloads.find((item) => ownsDownload(item) && activeDownloadStatuses.includes(item.status)),
  );

  const terminalDownload = $derived.by(() => {
    if (installed) return undefined;
    return $downloads.find(
      (item): item is DownloadItem & { status: TerminalDownloadStatus } =>
        ownsDownload(item) && (item.status === 'completed' || item.status === 'failed'),
    );
  });

  const ownInstall = $derived.by(() => {
    const ids = new Set($downloads.filter(ownsDownload).map((item) => item.id));
    return $installations.find(
      (item) =>
        installActive(item.status) && (ids.has(item.downloadId) || (localGame && item.gameId === localGame.id)),
    );
  });

  const busy = $derived(
    busyState([
      ownInstall ? { active: true, label: installStatusLabels[ownInstall.status], progress: ownInstall.progress } : null,
      update && (update.state === 'updating' || update.state === 'update_downloading')
        ? { active: true, label: stepLabels[update.step ?? 'download'], progress: update.progress }
        : null,
      ownDownload ? { active: true, label: statusLabels[ownDownload.status], progress: ownDownload.progress } : null,
    ]),
  );

  const availableGroups = $derived(releaseGroups.filter((group) => group.release.availability !== 'removed'));

  const primary = $derived(
    hubAction({
      installed,
      running,
      updateAvailable,
      releaseCount: availableGroups.length,
      releasesLoading,
      busy,
      terminalDownload: terminalDownload ? { status: terminalDownload.status } : null,
    }),
  );

  const busyPercent = $derived(Math.round((busy?.progress ?? 0) * 100));

  const menuItems = $derived([
    ...(localGame
      ? [
          localGame.completed
            ? { id: 'completed-unset', label: 'Снять отметку «Пройдена»' }
            : { id: 'completed-set', label: 'Отметить пройденной' },
        ]
      : []),
    ...($metadataAvailable && canonicalId
      ? metaView?.resolved
        ? [
            { id: 'meta-refresh', label: metaRefreshing ? 'Обновление…' : 'Обновить метаданные' },
            { id: 'meta-change', label: 'Сменить сопоставление' },
          ]
        : [{ id: 'meta-find', label: 'Найти метаданные' }]
      : []),
    ...(installed
      ? [
          localGame?.shortcutPath
            ? { id: 'shortcut-remove', label: 'Удалить ярлык с рабочего стола' }
            : { id: 'shortcut-create', label: 'Создать ярлык на рабочем столе' },
        ]
      : []),
    ...(installed ? [{ id: 'uninstall', label: 'Удалить с компьютера', danger: true, separator: true }] : []),
    ...(localGame
      ? [{ id: 'remove', label: 'Удалить из библиотеки', danger: true, separator: !installed }]
      : []),
    ...(terminalDownload
      ? [
          { id: 'remove-download', label: 'Удалить загрузку', danger: true, separator: true },
          { id: 'discard-download', label: 'Удалить загрузку и файлы', danger: true },
        ]
      : []),
  ]);

  function openMatch(mode: 'find' | 'change') {
    matchMode = mode;
    matchOpen = true;
  }

  function onMenu(actionId: string) {
    if (actionId === 'remove') {
      removeMode = 'library';
      removeOpen = true;
    } else if (actionId === 'uninstall') {
      removeMode = 'disk';
      removeOpen = true;
    } else if (actionId === 'meta-find') {
      openMatch('find');
    } else if (actionId === 'meta-change') {
      openMatch('change');
    } else if (actionId === 'meta-refresh') {
      refreshMeta();
    } else if (actionId === 'remove-download') {
      removeTerminalDownload();
    } else if (actionId === 'discard-download') {
      discardTerminalDownload();
    } else if (actionId === 'shortcut-create') {
      createDesktopShortcut();
    } else if (actionId === 'shortcut-remove') {
      removeDesktopShortcut();
    } else if (actionId === 'completed-set' || actionId === 'completed-unset') {
      const current = localGame;
      if (!current) return;
      void mark(() => setCompleted(current.id, actionId === 'completed-set'), 'Не удалось изменить отметку');
    }
  }

  async function mark(fn: () => Promise<unknown>, fallback: string) {
    try {
      await fn();
    } catch (err) {
      toast(markError(err, fallback), 'danger');
    }
  }

  function toggleFavorite() {
    const current = localGame;
    if (!current) return;
    void mark(() => setFavorite(current.id, !current.favorite), 'Не удалось изменить любимые');
  }

  async function createDesktopShortcut() {
    if (!localGame) return;
    try {
      await createShortcut(localGame.id);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось создать ярлык', 'danger');
    }
  }

  async function removeDesktopShortcut() {
    if (!localGame) return;
    try {
      await removeShortcut(localGame.id);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось удалить ярлык', 'danger');
    }
  }

  async function refreshMeta() {
    if (!canonicalId || metaRefreshing) return;
    metaRefreshing = true;
    try {
      metaView = await refreshMetadata(canonicalId);
      toast('Метаданные обновлены', 'success');
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось обновить метаданные', 'danger');
    } finally {
      metaRefreshing = false;
    }
  }

  async function skipMeta() {
    if (!canonicalId || metaSkipping) return;
    metaSkipping = true;
    try {
      metaView = await dismissMetadataMatch(canonicalId);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось отложить поиск метаданных', 'danger');
    } finally {
      metaSkipping = false;
    }
  }

  function openShot(index: number) {
    lightboxIndex = index;
    lightboxOpen = true;
  }

  async function loadReleases(canonicalGameId: string | undefined, gameTitle: string | undefined) {
    const current = ++releaseToken;
    releasesLoading = true;
    try {
      const groups = canonicalGameId
        ? await getReleasesForGame(canonicalGameId)
        : gameTitle
          ? await getReleasesForTitle(gameTitle)
          : [];
      if (current !== releaseToken) return;
      releaseGroups = groups;
    } catch {
      if (current !== releaseToken) return;
      releaseGroups = [];
    } finally {
      if (current === releaseToken) releasesLoading = false;
    }
  }

  $effect(() => {
    releaseKey;
    const canonicalGameId = canonicalId;
    const gameTitle = releaseTitle;
    untrack(() => {
      loadReleases(canonicalGameId, gameTitle);
    });
  });

  let downloadModalOpen = $state(false);
  let downloadSource = $state('');
  let downloadOrigin = $state<DownloadOrigin | undefined>(undefined);

  let installModalOpen = $state(false);
  let installModalDownloadId = $state<string | null>(null);

  async function retryTerminalDownload() {
    if (!terminalDownload) return;
    try {
      await resumeDownload(terminalDownload.id);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось повторить загрузку', 'danger');
    }
  }

  function installFromTerminalDownload() {
    if (!terminalDownload) return;
    installModalDownloadId = terminalDownload.id;
    installModalOpen = true;
  }

  function leaveWithoutCard() {
    if (!localGame) navigate('library');
  }

  async function removeTerminalDownload() {
    if (!terminalDownload) return;
    try {
      await removeDownload(terminalDownload.id);
      leaveWithoutCard();
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось удалить загрузку', 'danger');
    }
  }

  // Cancel purges the files only for a download that never completed:
  // manager.discard keeps a completed one, and DeleteData is the call that
  // removes its content.
  async function discardTerminalDownload() {
    if (!terminalDownload) return;
    const id = terminalDownload.id;
    const freesDisk = cancelFreesDisk(terminalDownload.status);
    try {
      await (freesDisk ? cancelDownload(id) : deleteDownloadData(id));
      leaveWithoutCard();
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось удалить загрузку и файлы', 'danger');
    }
  }

  async function downloadRelease(group: ReleaseGroup) {
    try {
      const request = await prepareReleaseDownload(group.release.id);
      downloadSource = request.uri;
      downloadOrigin = { releaseId: request.releaseId, sourceId: request.sourceId, gameId: request.gameId };
      downloadModalOpen = true;
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось подготовить загрузку', 'danger');
    }
  }

  function startInstall() {
    if (availableGroups.length === 0) return;
    if (availableGroups.length === 1) {
      downloadRelease(availableGroups[0]);
      return;
    }
    pickerOpen = true;
  }

  async function play() {
    if (!localGame) return;
    try {
      await playGame(localGame.id);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось запустить игру', 'danger');
    }
  }

  async function stop() {
    if (!localGame) return;
    try {
      await stopGame(localGame.id);
    } catch {
      toast('Не удалось остановить игру', 'danger');
    }
  }

  async function reveal() {
    if (!localGame) return;
    try {
      await openFolder(localGame.installDir);
    } catch {
      toast('Папка недоступна', 'danger');
    }
  }

  let addingToLibrary = $state(false);

  async function addToLibrary() {
    if (!canonicalId || addingToLibrary) return;
    addingToLibrary = true;
    try {
      await addCatalogGame(canonicalId, title, coverSrc);
      toast('Игра добавлена в библиотеку', 'success');
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось добавить игру в библиотеку', 'danger');
    } finally {
      addingToLibrary = false;
    }
  }

  function runPrimary() {
    if (primary.kind === 'play') play();
    else if (primary.kind === 'stop') stop();
    else if (primary.kind === 'install') startInstall();
    else if (primary.kind === 'update') updateCard?.start();
    else if (primary.kind === 'retry-download') retryTerminalDownload();
    else if (primary.kind === 'install-download') installFromTerminalDownload();
  }

  const missing = $derived(
    isGameMissing({
      hasLocalGame: Boolean(localGame),
      hasCatalogGame: Boolean(catalogGame),
      catalogLoading,
      hasOwnDownload: Boolean(anyOwnDownload),
    }),
  );
</script>

{#if missing}
  <EmptyState title="Игра не найдена" description="Возможно, она была удалена из библиотеки.">
    {#snippet actions()}
      <Button onclick={() => navigate('library')}>В библиотеку</Button>
    {/snippet}
  </EmptyState>
{:else}
  <section class="hero" class:plain={!showHero}>
    <div class="art">
      {#if showHero}
        <img src={heroSrc} alt="" draggable="false" onerror={() => (heroFailed = true)} />
      {/if}
    </div>
    <div class="veil"></div>

    <nav class="breadcrumb">
      {#if installed}
        <button class="crumb" onclick={() => navigate('installed')}>Установлено</button>
      {:else}
        <button class="crumb" onclick={() => navigate('library')}>Библиотека</button>
      {/if}
      <ChevronRight size="1.4rem" strokeWidth={1.8} />
      <span class="crumb current">{title || 'Игра'}</span>
    </nav>

    <div class="foot">
      <div class="cover">
        {#if title}
          <Artwork src={coverSrc} alt={title} ratio="3 / 4" radius="var(--radius-md)" />
        {:else}
          <div class="skeleton cover-skeleton"></div>
        {/if}
      </div>

      <div class="ident">
        <div class="state">
          {#if running}
            <StatusBadge kind="accent" label="Запущена" />
          {:else if installed}
            <StatusBadge kind="success" label="Установлено" dot={false} />
          {:else if localGame}
            <StatusBadge kind="neutral" label="Удалена с компьютера" dot={false} />
          {:else if title}
            <StatusBadge kind="neutral" label="Не установлено" dot={false} />
          {/if}
          {#if updateAvailable && !busy}
            <StatusBadge kind="accent" label="Доступно обновление" />
          {/if}
        </div>

        {#if title}
          <h1 class="title">{title}</h1>
        {:else}
          <div class="skeleton title-skeleton"></div>
        {/if}

        {#if metaParts.length > 0}
          <p class="metaline">
            {#each metaParts as part, index (part)}
              {#if index > 0}<span class="sep">·</span>{/if}
              <span>{part}</span>
            {/each}
          </p>
        {/if}

        <div class="cta">
          {#if primary.kind === 'progress'}
            <div class="progress">
              <span class="progress-label">{primary.label}</span>
              <ProgressBar value={busyPercent} />
              <span class="progress-pct">{busyPercent}%</span>
            </div>
          {:else}
            <Button variant="primary" size="lg" disabled={primary.disabled} onclick={runPrimary}>
              {#if primary.kind === 'play'}
                <Play size="1.6rem" strokeWidth={2} fill="currentColor" />
              {:else if primary.kind === 'stop'}
                <Square size="1.4rem" strokeWidth={2} fill="currentColor" />
              {:else if primary.kind === 'install' || primary.kind === 'install-download'}
                <Download size="1.6rem" strokeWidth={1.8} />
              {/if}
              {primary.label}
            </Button>
          {/if}

          {#if primary.kind === 'update'}
            <Button size="lg" onclick={play}>
              <Play size="1.5rem" strokeWidth={2} fill="currentColor" />
              Играть
            </Button>
          {/if}

          {#if !localGame && canonicalId}
            <Button size="lg" disabled={addingToLibrary} onclick={addToLibrary}>
              <BookmarkPlus size="1.5rem" strokeWidth={1.8} />
              Добавить в библиотеку
            </Button>
          {/if}

          {#if localGame}
            <IconButton
              label={localGame.favorite ? 'Убрать из любимых' : 'В любимые'}
              active={Boolean(localGame.favorite)}
              onclick={toggleFavorite}
            >
              <Heart size="1.8rem" strokeWidth={1.8} fill={localGame.favorite ? 'currentColor' : 'none'} />
            </IconButton>
          {/if}

          {#if menuItems.length > 0}
            <DropdownMenu items={menuItems} onselect={onMenu}>
              {#snippet trigger({ toggle })}
                <IconButton label="Ещё" onclick={toggle}>
                  <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
                </IconButton>
              {/snippet}
            </DropdownMenu>
          {/if}
        </div>

        {#if primary.kind === 'retry-download' && terminalDownload?.error}
          <p class="note danger">{terminalDownload.error}</p>
        {:else if localGame && !installed}
          <p class="note">Игра удалена с компьютера — установите её снова из доступных загрузок.</p>
        {/if}
      </div>
    </div>
  </section>

  <div class="body">
    <div class="main">
      {#if metaState === 'searching'}
        <div class="meta-note" role="status">
          <span class="spinner"></span>
          <div class="meta-text">
            <p class="meta-title">Ищем описание и обложку</p>
            <p class="meta-hint">Сопоставляем «{title || 'игру'}» с базой IGDB — это занимает несколько секунд.</p>
          </div>
        </div>
      {:else if metaState === 'unmatched' || metaState === 'failed'}
        <div class="meta-note">
          <div class="meta-text">
            <p class="meta-title">
              {metaState === 'unmatched' ? 'Не удалось подобрать описание' : 'Метаданные не загрузились'}
            </p>
            <p class="meta-hint">
              {metaState === 'unmatched'
                ? 'В базе IGDB нет однозначного совпадения. Выберите игру вручную или оставьте карточку как есть.'
                : 'Сервис метаданных не ответил. Попробуйте ещё раз или выберите игру вручную.'}
            </p>
          </div>
          <div class="meta-actions">
            {#if metaState === 'failed'}
              <Button size="sm" variant="primary" disabled={metaRefreshing} onclick={refreshMeta}>
                {metaRefreshing ? 'Повторяем…' : 'Повторить'}
              </Button>
              <Button size="sm" onclick={() => openMatch('find')}>Выбрать вручную</Button>
            {:else}
              <Button size="sm" variant="primary" onclick={() => openMatch('find')}>Выбрать вручную</Button>
            {/if}
            <Button size="sm" variant="ghost" disabled={metaSkipping} onclick={skipMeta}>Не искать</Button>
          </div>
        </div>
      {/if}

      {#if summary.text}
        <p class="summary">{summary.text}</p>
        {#if summary.expandable}
          <button class="more" onclick={() => (summaryExpanded = !summaryExpanded)}>
            {summaryExpanded ? 'Свернуть' : 'Показать полностью'}
          </button>
        {/if}
      {:else if !title || metaState === 'searching'}
        <div class="skeleton line"></div>
        <div class="skeleton line"></div>
        <div class="skeleton line short"></div>
      {/if}

      {#if tags.length > 0}
        <div class="tags">
          {#each tags as tag (tag)}
            <span class="tag">{tag}</span>
          {/each}
        </div>
      {/if}

      {#if shots.length > 0}
        <section class="section">
          <h2 class="heading">Скриншоты</h2>
          <div
            class="shots"
            class:featured={shots.length >= 5}
            class:one={shots.length === 1}
            style:--cols={shotCols}
          >
            {#each shots as shot, index (shot.id)}
              <button class="shot" onclick={() => openShot(index)} aria-label="Открыть скриншот">
                <Artwork src={shot.url ?? ''} alt="" ratio="16 / 9" radius="var(--radius-sm)" />
              </button>
            {/each}
          </div>
        </section>
      {/if}

      {#if showUpdateCard && update}
        <section class="section">
          <UpdateCard bind:this={updateCard} {update} {running} />
        </section>
      {/if}

      {#if installed && localGame}
        <section class="section">
          <VerifyCard gameId={localGame.id} state={verifyState} {running} />
        </section>
      {/if}

      {#if releasesLoading || releaseGroups.length > 0}
        <section class="section">
          <h2 class="heading">Доступные загрузки</h2>
          <ReleaseList
            groups={releaseGroups}
            loading={releasesLoading}
            currentReleaseId={localGame?.releaseId ?? ''}
            updateReleaseId={updateAvailable ? (update?.availability.targetReleaseId ?? '') : ''}
            ondownload={downloadRelease}
          />
        </section>
      {/if}
    </div>

    <aside class="side">
      {#if gameFacts.length > 0}
        <section class="side-block">
          <h2 class="heading sm">Об игре</h2>
          <dl class="facts">
            {#each gameFacts as fact (fact.label)}
              <div class="fact">
                <dt>{fact.label}</dt>
                <dd class:mono={fact.mono} title={fact.full ?? undefined}>{fact.value}</dd>
              </div>
            {/each}
          </dl>
        </section>
      {/if}

      {#if installed && localGame}
        <section class="panel">
          <h2 class="heading sm">Установка</h2>
          <div class="path">
            <span class="path-value" title={localGame.installDir}>{truncateMiddle(localGame.installDir, 34)}</span>
            <IconButton label="Открыть папку" size="sm" onclick={reveal}>
              <FolderOpen size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          </div>
          <dl class="facts">
            {#each installFacts as fact (fact.label)}
              <div class="fact">
                <dt>{fact.label}</dt>
                <dd class:mono={fact.mono} title={fact.full ?? undefined}>{fact.value}</dd>
              </div>
            {/each}
          </dl>
        </section>
      {/if}
    </aside>
  </div>

  {#if localGame}
    <RemoveGameModal
      bind:open={removeOpen}
      bind:mode={removeMode}
      gameId={localGame.id}
      title={localGame.title}
      onremoved={(removed) => {
        if (removed === 'library') navigate('installed');
      }}
    />
  {/if}

  <Modal bind:open={pickerOpen} title="Выберите загрузку" width="86rem">
    <ReleaseList
      groups={availableGroups}
      currentReleaseId={localGame?.releaseId ?? ''}
      ondownload={(group) => {
        pickerOpen = false;
        downloadRelease(group);
      }}
    />
  </Modal>

  <Lightbox
    bind:open={lightboxOpen}
    bind:index={lightboxIndex}
    items={shots.map((shot) => ({ id: shot.id, url: shot.url ?? '' }))}
    label={title}
  />

  <MetadataMatchModal
    bind:open={matchOpen}
    gameId={canonicalId ?? ''}
    gameTitle={title}
    mode={matchMode}
    onapplied={(view) => (metaView = view)}
  />

  <AddDownloadModal bind:open={downloadModalOpen} initialSource={downloadSource} origin={downloadOrigin} />
  <InstallModal bind:open={installModalOpen} downloadId={installModalDownloadId} />
{/if}

<style>
  .hero {
    position: relative;
    margin-inline: calc(var(--page-x) * -1);
    margin-bottom: var(--space-10);
    padding: var(--space-4) var(--page-x) var(--space-6);
    min-height: clamp(31rem, 42vh, 48rem);
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    gap: var(--space-8);
  }

  .art {
    position: absolute;
    inset: 0;
    overflow: hidden;
    background: var(--bg);
  }

  .art img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: 50% 28%;
    animation: art-in 320ms var(--ease);
  }

  .hero.plain {
    min-height: clamp(24rem, 28vh, 30rem);
  }

  .hero.plain .art {
    background:
      radial-gradient(100% 78% at 6% 0%, rgba(104, 117, 232, 0.2), transparent 64%),
      radial-gradient(80% 70% at 88% 8%, rgba(255, 255, 255, 0.05), transparent 62%),
      linear-gradient(180deg, var(--surface-3), var(--bg) 86%);
  }

  .veil {
    position: absolute;
    inset: 0;
    pointer-events: none;
    background:
      linear-gradient(
        180deg,
        rgba(11, 15, 20, 0.72) 0%,
        rgba(11, 15, 20, 0.22) 22%,
        rgba(11, 15, 20, 0.55) 52%,
        rgba(11, 15, 20, 0.88) 76%,
        var(--bg) 96%
      ),
      linear-gradient(
        90deg,
        rgba(11, 15, 20, 0.92) 0%,
        rgba(11, 15, 20, 0.68) 36%,
        rgba(11, 15, 20, 0.22) 68%,
        rgba(11, 15, 20, 0) 90%
      );
  }

  .hero.plain .veil {
    background: linear-gradient(180deg, transparent 60%, rgba(11, 15, 20, 0.6) 82%, var(--bg) 100%);
  }

  .breadcrumb {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.8rem;
    color: var(--text-3);
  }

  .crumb {
    font-size: var(--font-xs);
    color: var(--text-3);
    border-radius: var(--radius-sm);
    transition: color var(--dur) var(--ease);
  }

  button.crumb:hover {
    color: var(--text);
  }

  .crumb.current {
    color: var(--text-2);
    font-weight: 500;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .foot {
    position: relative;
    display: flex;
    align-items: flex-end;
    gap: var(--space-6);
  }

  .cover {
    width: clamp(11rem, 10vw, 16rem);
    flex-shrink: 0;
    margin-bottom: -3.6rem;
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
  }

  .cover-skeleton {
    aspect-ratio: 3 / 4;
    border-radius: var(--radius-md);
  }

  .ident {
    flex: 1;
    min-width: 0;
    padding-bottom: 0.4rem;
  }

  .state {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    flex-wrap: wrap;
    margin-bottom: 1rem;
    min-height: 2.4rem;
  }

  .title {
    font-size: clamp(2.8rem, 2.4vw + 1rem, 4.6rem);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.04;
    text-wrap: balance;
    max-width: 26ch;
    text-shadow: 0 0.2rem 2.4rem rgba(0, 0, 0, 0.5);
  }

  .title-skeleton {
    width: min(48rem, 60%);
    height: 4.4rem;
    border-radius: var(--radius-md);
  }

  .metaline {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.7rem;
    margin-top: 1rem;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .sep {
    color: var(--text-3);
  }

  .cta {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
    margin-top: var(--space-5);
  }

  .progress {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: min(42rem, 100%);
    height: var(--control-lg);
    padding: 0 1.8rem;
    border-radius: var(--cut) var(--radius-md) var(--radius-md) var(--radius-md);
    background: var(--surface-3);
  }

  .progress-label {
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
  }

  .progress :global(.track) {
    flex: 1;
  }

  .progress-pct {
    font-size: var(--font-xs);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .note {
    margin-top: var(--space-4);
    max-width: 56rem;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .note.danger {
    color: var(--danger);
  }

  .body {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 30rem;
    gap: var(--space-12);
    align-items: start;
  }

  .main {
    min-width: 0;
  }

  .main > :first-child {
    margin-top: 0;
  }

  .meta-note {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex-wrap: wrap;
    max-width: var(--prose-max);
    margin-bottom: var(--space-6);
    padding: var(--space-4) var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  .meta-text {
    flex: 1;
    min-width: 22rem;
  }

  .meta-title {
    font-size: var(--font-sm);
    font-weight: 600;
    color: var(--text);
  }

  .meta-hint {
    margin-top: 0.4rem;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }

  .meta-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .spinner {
    width: 2rem;
    height: 2rem;
    flex-shrink: 0;
    border-radius: 50%;
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    animation: spin 800ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .summary {
    max-width: var(--prose-max);
    font-size: var(--font-md);
    line-height: 1.65;
    color: var(--text-2);
    white-space: pre-line;
  }

  .more {
    margin-top: 1rem;
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--accent-text);
    border-radius: var(--radius-sm);
  }

  .more:hover {
    text-decoration: underline;
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
    margin-top: var(--space-5);
  }

  .tag {
    display: inline-flex;
    align-items: center;
    height: 2.4rem;
    padding: 0 0.9rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    color: var(--text-2);
    font-size: var(--font-xs);
    white-space: nowrap;
  }

  .section {
    margin-top: var(--space-10);
  }

  .heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  .heading.sm {
    font-size: var(--font-md);
    color: var(--text-2);
    margin-bottom: var(--space-2);
  }

  .shots {
    display: grid;
    grid-template-columns: repeat(var(--cols, 3), minmax(0, 1fr));
    gap: var(--space-3);
  }

  .shots.one {
    max-width: 72rem;
  }

  .shot {
    display: block;
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: var(--radius-sm);
    overflow: hidden;
    transition:
      transform var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }

  .shot:hover {
    transform: translateY(-2px);
    opacity: 0.92;
  }

  .shots.featured .shot:first-child {
    grid-column: span 2;
    grid-row: span 2;
  }

  .side {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
  }

  .panel {
    padding: var(--space-5);
    background: var(--surface);
    border-radius: var(--radius-lg);
  }

  .path {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: var(--space-2);
  }

  .path-value {
    flex: 1;
    min-width: 0;
    font-size: var(--font-xs);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .facts {
    display: flex;
    flex-direction: column;
  }

  .fact {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.9rem 0;
    border-top: 1px solid var(--border);
  }

  dt {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  dd {
    font-size: var(--font-sm);
    color: var(--text);
    min-width: 0;
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    font-variant-numeric: tabular-nums;
  }

  dd.mono {
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .skeleton {
    background: linear-gradient(90deg, var(--surface-2), var(--surface-3), var(--surface-2));
    background-size: 200% 100%;
    animation: shimmer 1.4s linear infinite;
    border-radius: var(--radius-sm);
  }

  .line {
    height: 1.6rem;
    margin-bottom: 1rem;
    max-width: var(--prose-max);
  }

  .line.short {
    width: 40%;
  }

  @keyframes shimmer {
    to {
      background-position: -200% 0;
    }
  }

  @keyframes art-in {
    from {
      opacity: 0;
      transform: scale(1.02);
    }
  }

  @media (max-width: 1400px) {
    .body {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--space-8);
    }

    .side {
      flex-direction: row;
      flex-wrap: wrap;
      gap: var(--space-8);
      margin-top: var(--space-10);
    }

    .side > :global(*) {
      flex: 1 1 32rem;
    }

    .shots {
      grid-template-columns: repeat(auto-fill, minmax(28rem, 1fr));
    }

    .shots.featured {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 1200px) {
    .hero {
      min-height: 28rem;
      gap: var(--space-6);
    }

    .foot {
      gap: var(--space-4);
    }

    .cover {
      width: 9rem;
      margin-bottom: -2.4rem;
    }

    .shots,
    .shots.featured {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .shots.featured .shot:first-child {
      grid-row: span 1;
    }
  }
</style>
