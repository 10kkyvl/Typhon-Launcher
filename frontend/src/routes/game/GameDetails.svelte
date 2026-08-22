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
  import { untrack } from 'svelte';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameHero from '../../lib/components/GameHero.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import ReleaseList from '../../lib/components/ReleaseList.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import UpdateCard from '../../lib/components/UpdateCard.svelte';
  import VerifyCard from '../../lib/components/VerifyCard.svelte';
  import type { DownloadOrigin } from '../../lib/services/downloads';
  import { playGame, removeGame, stopGame } from '../../lib/services/library';
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
  import { updatesByGame, verifications } from '../../lib/stores/updates';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { bytesLabel, gb, playtime, relativeDate } from '../../lib/utils/format';

  let { id }: { id: string } = $props();

  const localGame = $derived($libraryGames.find((g) => g.id === id));
  const localRunning = $derived(localGame ? $runningGames.has(localGame.id) : false);

  let removeOpen = $state(false);

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

  async function localRemove() {
    if (!localGame) return;
    const title = localGame.title;
    try {
      await removeGame(localGame.id);
      removeOpen = false;
      toast(`«${title}» удалена из библиотеки`);
      navigate('installed');
    } catch {
      toast('Не удалось удалить игру', 'danger');
    }
  }
</script>
{#if localGame}
  <nav class="breadcrumb">
    <button class="crumb" onclick={() => navigate('installed')}>Установлено</button>
    <ChevronRight size="1.4rem" strokeWidth={1.8} />
    <span class="crumb current">{localGame.title}</span>
  </nav>

  <section class="local-hero">
    {#if localGame.hero}
      <div class="local-art">
        <Artwork src={localGame.hero} alt="" />
      </div>
    {/if}
    <div class="local-content">
      <div class="local-head">
        <h1 class="local-title">{localGame.title}</h1>
        {#if localRunning}
          <StatusBadge kind="accent" label="Запущена" />
        {:else}
          <StatusBadge kind="success" label="Установлено" dot={false} />
        {/if}
      </div>
      <p class="local-path">{localGame.installDir}</p>
      <div class="actions">
        {#if localRunning}
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
        <Button size="lg" onclick={localOpenFolder}>
          <FolderOpen size="1.6rem" strokeWidth={1.8} />
          Открыть папку
        </Button>
        <DropdownMenu
          items={[{ id: 'remove', label: 'Удалить из библиотеки', danger: true }]}
          onselect={(actionId) => {
            if (actionId === 'remove') removeOpen = true;
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
  </section>

  <div class="local-grid">
    <div class="local-col">
      <h3 class="group-title">Сведения</h3>
      <dl class="local-props">
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
        <div class="prop">
          <dt>Состояние</dt>
          <dd>{localRunning ? 'Запущена' : 'Не запущена'}</dd>
        </div>
      </dl>
    </div>
  </div>

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

  <Modal bind:open={removeOpen} title="Удалить игру из библиотеки?">
    <p class="modal-text">
      «{localGame.title}» будет убрана из библиотеки Typhon. Файлы в папке {localGame.installDir} останутся на месте.
    </p>
    {#snippet footer()}
      <Button onclick={() => (removeOpen = false)}>Отмена</Button>
      <Button variant="danger" onclick={localRemove}>Удалить</Button>
    {/snippet}
  </Modal>

  <AddDownloadModal bind:open={downloadModalOpen} initialSource={downloadSource} origin={downloadOrigin} />
{:else if catalogGame}
  <nav class="breadcrumb">
    <button class="crumb" onclick={() => navigate('library')}>Библиотека</button>
    <ChevronRight size="1.4rem" strokeWidth={1.8} />
    <span class="crumb current">{catalogGame.title}</span>
  </nav>

  <section class="local-hero">
    <div class="local-content">
      <div class="local-head">
        <h1 class="local-title">{catalogGame.title}</h1>
        <StatusBadge kind="neutral" label="Не установлено" dot={false} />
      </div>
      {#if catalogMeta}
        <p class="local-path">{catalogMeta}</p>
      {/if}
    </div>
  </section>

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

  .modal-text {
    font-size: var(--font-md);
    line-height: 1.55;
    color: var(--text-2);
  }

  @media (max-width: 1100px) {
    .local-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
