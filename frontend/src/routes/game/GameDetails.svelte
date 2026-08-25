<script lang="ts">
  import {
    Bookmark,
    Calendar,
    Check,
    ChevronDown,
    ChevronRight,
    CircleCheck,
    Download,
    EllipsisVertical,
    ExternalLink,
    FileCheck,
    FolderOpen,
    Gamepad2,
    Globe,
    Image,
    NotebookPen,
    Play,
    Settings,
    Square,
    SquarePen,
    Terminal,
    Trash2,
    Trophy,
    UploadCloud,
    UserRound,
  } from '@lucide/svelte';
  import { onMount, untrack } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameHero from '../../lib/components/GameHero.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import MetadataMatchModal from '../../lib/components/MetadataMatchModal.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import ReleaseList from '../../lib/components/ReleaseList.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import UpdateCard from '../../lib/components/UpdateCard.svelte';
  import VerifyCard from '../../lib/components/VerifyCard.svelte';
  import type { DownloadOrigin } from '../../lib/services/downloads';
  import { playGame, stopGame } from '../../lib/services/library';
  import { ensureMetadataFresh, getMetadataView, refreshMetadata, type MetadataView } from '../../lib/services/metadata';
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
  import { libraryGames, runningGames } from '../../lib/stores/library';
  import { metadataAvailable } from '../../lib/stores/metadata';
  import { updatesByGame, verifications } from '../../lib/stores/updates';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { bytesLabel, gb, playtime, relativeDate } from '../../lib/utils/format';

  let { id }: { id: string } = $props();

  const localGame = $derived($libraryGames.find((g) => g.id === id));
  const localInstalled = $derived(Boolean(localGame) && !localGame?.uninstalled);
  const localRunning = $derived(localGame ? $runningGames.has(localGame.id) : false);

  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('library');

  const localUpdate = $derived(localGame ? $updatesByGame.get(localGame.id) : undefined);
  const localVerify = $derived(localGame ? $verifications[localGame.id] : undefined);
  const showUpdateCard = $derived(
    Boolean(
      localUpdate &&
        (localUpdate.availability.available ||
          localUpdate.canRollback ||
          localUpdate.state === 'updating' ||
          localUpdate.state === 'update_downloading' ||
          localUpdate.state === 'update_failed'),
    ),
  );

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
  let catalogToken = 0;

  async function loadCatalogGame(gameId: string) {
    const current = ++catalogToken;
    const found = await getCatalogGame(gameId);
    if (current !== catalogToken) return;
    catalogGame = found;
  }

  $effect(() => {
    const gameId = id;
    const known = Boolean(localGame);
    untrack(() => {
      if (known) {
        catalogToken++;
        catalogGame = null;
        return;
      }
      loadCatalogGame(gameId);
    });
  });

  const canonicalId = $derived(localGame?.canonicalGameId || catalogGame?.id);
  const catalogMeta = $derived(
    [catalogGame?.releaseYear, catalogGame?.developer, catalogGame?.publisher].filter(Boolean).join(' · '),
  );
  const releaseTitle = $derived(localGame?.title ?? catalogGame?.title);
  const releaseKey = $derived(`${id}|${canonicalId ?? ''}|${releaseTitle ?? ''}`);
  const pageTitle = $derived(localGame?.title ?? catalogGame?.title ?? '');

  let metaView = $state<MetadataView | null>(null);
  let metaToken = 0;
  let metaRefreshing = $state(false);
  let matchOpen = $state(false);
  let matchMode = $state<'find' | 'change'>('find');
  let summaryExpanded = $state(false);
  let screenshotOpen = $state(false);
  let screenshotSrc = $state('');

  async function loadMetaView(gameId: string) {
    const current = ++metaToken;
    const view = await getMetadataView(gameId);
    if (current !== metaToken) return;
    metaView = view;
  }

  $effect(() => {
    const metaGameId = canonicalId;
    untrack(() => {
      if (!metaGameId) {
        metaToken++;
        metaView = null;
        return;
      }
      loadMetaView(metaGameId);
      ensureMetadataFresh(metaGameId);
    });
  });

  onMount(() => {
    return Events.On('metadata:updated', (event) => {
      const view = event.data as MetadataView;
      if (view.game?.id && view.game.id === canonicalId) {
        metaView = view;
      }
    });
  });

  const metaCover = $derived(metaView?.cover ?? '');
  const metaHero = $derived(metaView?.hero ?? '');
  const heroSrc = $derived(metaHero);
  const summary = $derived(metaView?.game.summary ?? '');
  const summaryTooLong = $derived(summary.length > 600);
  const summaryShown = $derived(summaryExpanded || !summaryTooLong ? summary : `${summary.slice(0, 600)}…`);
  const releaseDateLabel = $derived.by(() => {
    const game = metaView?.game;
    if (!game) return '';
    if (game.releaseDate) {
      const date = new Date(game.releaseDate);
      if (!Number.isNaN(date.getTime())) return date.toLocaleDateString('ru-RU');
    }
    if (game.releaseYear) return String(game.releaseYear);
    return '';
  });
  const developerLabel = $derived(metaView?.game.developer ?? '');
  const publisherLabel = $derived(metaView?.game.publisher ?? '');
  const genres = $derived(metaView?.game.genres ?? []);
  const themes = $derived(metaView?.game.themes ?? []);
  const platforms = $derived(metaView?.game.platforms ?? []);
  const screenshots = $derived(metaView?.screenshots ?? []);

  function openMatch(mode: 'find' | 'change') {
    matchMode = mode;
    matchOpen = true;
  }

  function onMetaApplied(view: MetadataView) {
    metaView = view;
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

  function openScreenshot(url: string) {
    if (!url) return;
    screenshotSrc = url;
    screenshotOpen = true;
  }

  async function loadReleases(canonicalGameId: string | undefined, title: string | undefined) {
    const current = ++releaseToken;
    releasesLoading = true;
    try {
      const groups = canonicalGameId
        ? await getReleasesForGame(canonicalGameId)
        : title
          ? await getReleasesForTitle(title)
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
    const title = releaseTitle;
    untrack(() => {
      loadReleases(canonicalGameId, title);
    });
  });

  let downloadModalOpen = $state(false);
  let downloadSource = $state('');
  let downloadOrigin = $state<DownloadOrigin | undefined>(undefined);

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

  async function localPlay() {
    if (!localGame) return;
    try {
      await playGame(localGame.id);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось запустить игру', 'danger');
    }
  }

  async function localStop() {
    if (!localGame) return;
    try {
      await stopGame(localGame.id);
    } catch {
      toast('Не удалось остановить игру', 'danger');
    }
  }

  async function localOpenFolder() {
    if (!localGame) return;
    try {
      await openFolder(localGame.installDir);
    } catch {
      toast('Папка недоступна', 'danger');
    }
  }

</script>
{#if localGame}
  <nav class="breadcrumb">
    {#if localInstalled}
      <button class="crumb" onclick={() => navigate('installed')}>Установлено</button>
    {:else}
      <button class="crumb" onclick={() => navigate('library')}>Библиотека</button>
    {/if}
    <ChevronRight size="1.4rem" strokeWidth={1.8} />
    <span class="crumb current">{localGame.title}</span>
  </nav>

  <section class="local-hero">
    {#if heroSrc}
      <div class="local-art">
        <Artwork src={heroSrc} alt="" />
      </div>
    {/if}
    <div class="local-content">
      <div class="local-row">
        <div class="poster">
          <Artwork src={metaCover} alt={localGame.title} ratio="3 / 4" radius="var(--radius-md)" />
        </div>
        <div class="local-main">
          <div class="local-head">
            <h1 class="local-title">{localGame.title}</h1>
            {#if !localInstalled}
              <StatusBadge kind="neutral" label="Не установлено" dot={false} />
            {:else if localRunning}
              <StatusBadge kind="accent" label="Запущена" />
            {:else}
              <StatusBadge kind="success" label="Установлено" dot={false} />
            {/if}
          </div>
          {#if localInstalled}
            <p class="local-path">{localGame.installDir}</p>
          {/if}
          <div class="actions">
            {#if !localInstalled}
              <span class="local-note">Игра удалена с компьютера — установите её снова из доступных загрузок.</span>
            {:else if localRunning}
              <Button size="lg" onclick={localStop}>
                <Square size="1.5rem" strokeWidth={2} fill="currentColor" />
                Остановить
              </Button>
            {:else}
              <Button variant="primary" size="lg" onclick={localPlay}>
                <Play size="1.6rem" strokeWidth={2} fill="currentColor" />
                Играть
              </Button>
            {/if}
            {#if localInstalled}
              <Button size="lg" onclick={localOpenFolder}>
                <FolderOpen size="1.6rem" strokeWidth={1.8} />
                Открыть папку
              </Button>
            {/if}
            <DropdownMenu
              items={[
                ...($metadataAvailable
                  ? metaView?.resolved
                    ? [
                        { id: 'meta-refresh', label: metaRefreshing ? 'Обновление…' : 'Обновить метаданные' },
                        { id: 'meta-change', label: 'Сменить сопоставление' },
                      ]
                    : [{ id: 'meta-find', label: 'Найти метаданные' }]
                  : []),
                ...(localInstalled
                  ? [{ id: 'uninstall', label: 'Удалить с компьютера', danger: true, separator: $metadataAvailable }]
                  : []),
                {
                  id: 'remove',
                  label: 'Удалить из библиотеки',
                  danger: true,
                  separator: $metadataAvailable && !localInstalled,
                },
              ]}
              onselect={(actionId) => {
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
                }
              }}
            >
              {#snippet trigger({ toggle })}
                <IconButton label="Ещё" onclick={toggle}>
                  <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
                </IconButton>
              {/snippet}
            </DropdownMenu>
          </div>
        </div>
      </div>
    </div>
  </section>

  <div class="local-grid">
    <div class="local-col">
      <h3 class="group-title">Сведения</h3>
      <dl class="local-props">
        {#if localInstalled}
          <div class="prop">
            <dt>Исполняемый файл</dt>
            <dd class="mono">{localGame.executable}</dd>
          </div>
          <div class="prop">
            <dt>Версия</dt>
            <dd>{localGame.version || 'Неизвестна'}</dd>
          </div>
          <div class="prop">
            <dt>Размер</dt>
            <dd>{bytesLabel(localGame.sizeBytes)}</dd>
          </div>
        {:else}
          <div class="prop">
            <dt>Состояние</dt>
            <dd>Удалена с компьютера</dd>
          </div>
        {/if}
        <div class="prop">
          <dt>Добавлена</dt>
          <dd>{relativeDate(localGame.installedAt)}</dd>
        </div>
      </dl>
    </div>
    <div class="local-col">
      <h3 class="group-title">Активность</h3>
      <dl class="local-props">
        <div class="prop">
          <dt>Наиграно</dt>
          <dd>{playtime(localGame.playtimeSeconds)}</dd>
        </div>
        <div class="prop">
          <dt>Последний запуск</dt>
          <dd>{relativeDate(localGame.lastPlayed)}</dd>
        </div>
        {#if localInstalled}
          <div class="prop">
            <dt>Состояние</dt>
            <dd>{localRunning ? 'Запущена' : 'Не запущена'}</dd>
          </div>
        {/if}
      </dl>
    </div>
  </div>

  {@render metaBlocks()}

  {#if showUpdateCard && localUpdate}
    <UpdateCard update={localUpdate} running={localRunning} />
  {/if}

  <VerifyCard gameId={localGame.id} state={localVerify} running={localRunning} />

  {#if releasesLoading || releaseGroups.length > 0}
    <section class="block">
      <h2 class="section-title">Доступные загрузки</h2>
      <ReleaseList groups={releaseGroups} loading={releasesLoading} ondownload={downloadRelease} />
    </section>
  {/if}

  <RemoveGameModal
    bind:open={removeOpen}
    bind:mode={removeMode}
    gameId={localGame.id}
    title={localGame.title}
    onremoved={(removed) => {
      if (removed === 'library') navigate('installed');
    }}
  />

  <AddDownloadModal bind:open={downloadModalOpen} initialSource={downloadSource} origin={downloadOrigin} />
{:else if catalogGame}
  <nav class="breadcrumb">
    <button class="crumb" onclick={() => navigate('library')}>Библиотека</button>
    <ChevronRight size="1.4rem" strokeWidth={1.8} />
    <span class="crumb current">{catalogGame.title}</span>
  </nav>

  <section class="local-hero">
    {#if heroSrc}
      <div class="local-art">
        <Artwork src={heroSrc} alt="" />
      </div>
    {/if}
    <div class="local-content">
      <div class="local-row">
        <div class="poster">
          <Artwork src={metaCover} alt={catalogGame.title} ratio="3 / 4" radius="var(--radius-md)" />
        </div>
        <div class="local-main">
          <div class="local-head">
            <h1 class="local-title">{catalogGame.title}</h1>
            <StatusBadge kind="neutral" label="Не установлено" dot={false} />
          </div>
          {#if catalogMeta}
            <p class="local-path">{catalogMeta}</p>
          {/if}
          {#if $metadataAvailable}
            <div class="actions">
              {#if metaView?.resolved}
                <Button disabled={metaRefreshing} onclick={refreshMeta}>
                  {metaRefreshing ? 'Обновление…' : 'Обновить метаданные'}
                </Button>
                <Button onclick={() => openMatch('change')}>Сменить сопоставление</Button>
              {:else}
                <Button variant="primary" onclick={() => openMatch('find')}>Найти метаданные</Button>
              {/if}
            </div>
          {/if}
        </div>
      </div>
    </div>
  </section>

  {@render metaBlocks()}

  <section class="block">
    <h2 class="section-title">Доступные загрузки</h2>
    {#if releasesLoading || releaseGroups.length > 0}
      <ReleaseList groups={releaseGroups} loading={releasesLoading} ondownload={downloadRelease} />
    {:else}
      <p class="muted">Ни один источник не предлагает релизы для этой игры.</p>
    {/if}
  </section>

  <AddDownloadModal bind:open={downloadModalOpen} initialSource={downloadSource} origin={downloadOrigin} />
{:else}
  <EmptyState title="Игра не найдена" description="Возможно, она была удалена из библиотеки.">
    {#snippet actions()}
      <Button onclick={() => navigate('library')}>В библиотеку</Button>
    {/snippet}
  </EmptyState>
{/if}

{#snippet metaBlocks()}
  {#if summary}
    <section class="block">
      <h2 class="section-title">Об игре</h2>
      <p class="summary">{summaryShown}</p>
      {#if summaryTooLong}
        <button class="link-btn" onclick={() => (summaryExpanded = !summaryExpanded)}>
          {summaryExpanded ? 'Свернуть' : 'Показать полностью'}
        </button>
      {/if}
    </section>
  {/if}

  {#if releaseDateLabel || developerLabel || publisherLabel}
    <section class="block">
      <h2 class="section-title">Сведения об игре</h2>
      <dl class="local-props">
        {#if releaseDateLabel}
          <div class="prop">
            <dt>Дата выхода</dt>
            <dd>{releaseDateLabel}</dd>
          </div>
        {/if}
        {#if developerLabel}
          <div class="prop">
            <dt>Разработчик</dt>
            <dd>{developerLabel}</dd>
          </div>
        {/if}
        {#if publisherLabel}
          <div class="prop">
            <dt>Издатель</dt>
            <dd>{publisherLabel}</dd>
          </div>
        {/if}
      </dl>
    </section>
  {/if}

  {#if genres.length > 0 || themes.length > 0 || platforms.length > 0}
    <section class="block">
      <h2 class="section-title">Теги</h2>
      <div class="chips">
        {#each genres as genre (genre)}
          <span class="chip">{genre}</span>
        {/each}
        {#each themes as theme (theme)}
          <span class="chip">{theme}</span>
        {/each}
        {#each platforms as platform (platform)}
          <span class="chip">{platform}</span>
        {/each}
      </div>
    </section>
  {/if}

  {#if screenshots.length > 0}
    <section class="block">
      <h2 class="section-title">Скриншоты</h2>
      <div class="screens">
        {#each screenshots as shot (shot.id)}
          <button class="screen" onclick={() => openScreenshot(shot.url ?? '')}>
            <Artwork src={shot.url ?? ''} alt="" ratio="16 / 9" radius="var(--radius-sm)" />
          </button>
        {/each}
      </div>
    </section>
  {/if}
{/snippet}

<MetadataMatchModal bind:open={matchOpen} gameId={canonicalId ?? ''} gameTitle={pageTitle} mode={matchMode} onapplied={onMetaApplied} />

<Modal bind:open={screenshotOpen} title="Скриншот" width="fit-content">
  {#if screenshotSrc}
    <img class="screenshot-full" src={screenshotSrc} alt="" />
  {/if}
</Modal>

<style>
  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    margin: var(--space-3) 0 var(--space-4);
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
    color: var(--text);
    font-weight: 500;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-5);
    flex-wrap: wrap;
  }

  .block {
    margin-bottom: var(--space-8);
  }

  .section-title {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  dt {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  dd {
    font-size: var(--font-sm);
    color: var(--text);
    min-width: 0;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
    margin-top: var(--space-3);
  }

  @media (max-width: 1400px) {
  }

  .local-hero {
    position: relative;
    padding: var(--space-4) 0 var(--space-6);
  }

  .local-art {
    position: absolute;
    inset: 0 0 auto 0;
    height: 36rem;
    z-index: 0;
    pointer-events: none;
    opacity: 0.35;
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.9), transparent);
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.9), transparent);
  }

  .local-content {
    position: relative;
    z-index: 1;
  }

  .local-row {
    display: flex;
    align-items: flex-end;
    gap: var(--space-6);
  }

  .poster {
    width: 15rem;
    flex-shrink: 0;
  }

  .local-main {
    flex: 1;
    min-width: 0;
  }

  .local-head {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex-wrap: wrap;
  }

  .local-title {
    font-size: var(--font-hero);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.05;
    min-width: 0;
  }

  .local-note {
    max-width: 60rem;
    color: var(--text-3);
    line-height: 1.5;
  }

  .local-path {
    margin-top: 0.8rem;
    font-size: var(--font-sm);
    color: var(--text-3);
    word-break: break-all;
  }

  .local-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0 var(--space-12);
    max-width: 120rem;
    margin-bottom: var(--space-8);
  }

  .local-col {
    min-width: 0;
  }

  .group-title {
    font-size: var(--font-lg);
    font-weight: 600;
    margin-bottom: var(--space-2);
  }

  .local-props .prop {
    display: grid;
    grid-template-columns: 16rem 1fr;
    gap: var(--space-3);
    padding: 1rem 0;
    border-top: 1px solid var(--border);
  }

  .local-props dd {
    font-variant-numeric: tabular-nums;
  }

  .mono {
    word-break: break-all;
  }

  .summary {
    max-width: 90rem;
    font-size: var(--font-sm);
    line-height: 1.6;
    color: var(--text-2);
    white-space: pre-line;
  }

  .link-btn {
    margin-top: 0.8rem;
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--accent-text);
    border-radius: var(--radius-sm);
  }

  .link-btn:hover {
    text-decoration: underline;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.7rem;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    height: 2.6rem;
    padding: 0 1.1rem;
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: var(--font-xs);
    font-weight: 500;
    white-space: nowrap;
  }

  .screens {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(34rem, 1fr));
    gap: var(--space-4);
  }

  .screen {
    display: block;
    width: 100%;
    border-radius: var(--radius-sm);
    overflow: hidden;
    transition: transform var(--dur) var(--ease);
  }

  .screen:hover {
    transform: scale(1.015);
  }

  .screenshot-full {
    display: block;
    width: auto;
    height: auto;
    max-width: min(150rem, calc(100vw - 9.6rem));
    max-height: calc(100vh - 17rem);
    border-radius: var(--radius-md);
  }

  @media (max-width: 1100px) {
    .local-grid {
      grid-template-columns: minmax(0, 1fr);
    }

    .local-row {
      align-items: flex-start;
    }

    .poster {
      width: 10rem;
    }
  }
</style>
