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
  import { achievements, dlcs } from '../../lib/mock/achievements';
  import { gameById } from '../../lib/mock/games';
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

  const game = $derived(gameById(id));
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
    const known = Boolean(localGame || game);
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
  const releaseTitle = $derived(localGame?.title ?? catalogGame?.title ?? game?.title);
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
  const gameAchievements = $derived(achievements[id]);
  const gameDlcs = $derived(dlcs.filter((d) => d.gameId === id));

  let tab = $state('overview');
  let expanded = $state(false);
  let bookmarked = $state(false);

  const tabs = [
    { id: 'overview', label: 'Обзор' },
    { id: 'addons', label: 'Дополнения' },
    { id: 'achievements', label: 'Достижения' },
    { id: 'screenshots', label: 'Скриншоты' },
    { id: 'notes', label: 'Заметки' },
  ];

  const quickActions = [
    { id: 'settings', label: 'Параметры игры', sub: 'Графика, язык и др.', icon: Settings },
    { id: 'saves', label: 'Резервные копии', sub: 'Управление сохранениями', icon: UploadCloud },
    { id: 'verify', label: 'Проверить файлы', sub: 'Проверка целостности', icon: FileCheck },
    { id: 'folder', label: 'Открыть папку', sub: 'Папка с игрой', icon: FolderOpen },
    { id: 'args', label: 'Аргументы запуска', sub: 'Параметры командной строки', icon: Terminal },
    { id: 'uninstall', label: 'Удалить игру', sub: 'Освободить место', icon: Trash2, danger: true },
  ];

  function quickAction(actionId: string) {
    if (actionId === 'uninstall') toast('Удаление недоступно в demo', 'danger');
    else if (actionId === 'verify') toast('Проверка файлов запущена');
    else toast('Действие недоступно в demo');
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
{:else if !game}
  <EmptyState title="Игра не найдена" description="Возможно, она была удалена из библиотеки.">
    {#snippet actions()}
      <Button onclick={() => navigate('library')}>В библиотеку</Button>
    {/snippet}
  </EmptyState>
{:else}
  <nav class="breadcrumb">
    <button class="crumb" onclick={() => navigate('library')}>Библиотека</button>
    <ChevronRight size="1.4rem" strokeWidth={1.8} />
    <span class="crumb current">{game.title}</span>
  </nav>

  <GameHero src={game.hero} alt={game.title} ratio="2.6 / 1" minHeight="30rem" maxHeight="44rem">
    <h1 class="title">{game.title}</h1>
    <span class="genres">{game.genres.join(' · ')}</span>
    <div class="meta">
      <span class="meta-item">
        <Calendar size="1.5rem" strokeWidth={1.8} />
        <span class="meta-label">Выпуск</span>
        {game.releaseDate}
      </span>
      <span class="meta-item">
        <UserRound size="1.5rem" strokeWidth={1.8} />
        <span class="meta-label">Разработчик</span>
        {game.developer}
      </span>
      <span class="meta-item">
        <Globe size="1.5rem" strokeWidth={1.8} />
        <span class="meta-label">Язык</span>
        {game.language}
      </span>
    </div>
    <p class="tagline">{game.tagline}</p>
    <div class="actions">
      {#if game.installed}
        <Button variant="primary" size="lg" onclick={() => toast(`Запуск «${game.title}»...`)}>
          <Play size="1.6rem" strokeWidth={2} fill="currentColor" />
          Играть
        </Button>
        <DropdownMenu
          align="left"
          items={[
            { id: 'update', label: 'Проверить обновления' },
            { id: 'shortcut', label: 'Создать ярлык' },
            { id: 'uninstall', label: 'Удалить игру', danger: true, separator: true },
          ]}
          onselect={quickAction}
        >
          {#snippet trigger({ toggle })}
            <span class="installed-group">
              <Button size="lg">
                <Check size="1.6rem" strokeWidth={2} />
                Установлено
              </Button>
              <Button size="lg" onclick={toggle}>
                <ChevronDown size="1.6rem" strokeWidth={1.8} />
              </Button>
            </span>
          {/snippet}
        </DropdownMenu>
      {:else}
        <Button variant="primary" size="lg" onclick={() => toast(`«${game.title}» добавлена в очередь загрузки`)}>
          <Download size="1.6rem" strokeWidth={2} />
          Установить
        </Button>
        <span class="size-hint">{gb(game.sizeGb)}</span>
      {/if}
      <IconButton label="В коллекцию" active={bookmarked} onclick={() => (bookmarked = !bookmarked)}>
        <Bookmark size="1.8rem" strokeWidth={1.8} fill={bookmarked ? 'currentColor' : 'none'} />
      </IconButton>
    </div>
  </GameHero>

  <div class="tabs-row">
    <Tabs {tabs} bind:value={tab} />
  </div>

  {#if tab === 'overview'}
    <div class="overview">
      <div class="overview-main">
        <section class="block">
          <h2 class="section-title">О игре</h2>
          <p class="description">{game.description}</p>
          <dl class="props">
            <div class="prop">
              <dt>Режимы игры</dt>
              <dd>{game.modes.join(', ')}</dd>
            </div>
            <div class="prop">
              <dt>Поддержка контроллера</dt>
              <dd>{game.controllerSupport}</dd>
            </div>
            <div class="prop">
              <dt>Последнее обновление</dt>
              <dd>{game.lastUpdate}</dd>
            </div>
            {#if expanded}
              <div class="prop">
                <dt>Издатель</dt>
                <dd>{game.publisher}</dd>
              </div>
              <div class="prop">
                <dt>Версия</dt>
                <dd>{game.version}</dd>
              </div>
              <div class="prop">
                <dt>Размер</dt>
                <dd>{gb(game.sizeGb)}</dd>
              </div>
              {#if game.playtimeHours > 0}
                <div class="prop">
                  <dt>Наиграно</dt>
                  <dd>{game.playtimeHours} ч</dd>
                </div>
              {/if}
            {/if}
          </dl>
          <button class="expand" onclick={() => (expanded = !expanded)}>
            {expanded ? 'Показать меньше' : 'Показать больше'}
            <ChevronDown size="1.5rem" strokeWidth={1.8} style={expanded ? 'transform: rotate(180deg)' : ''} />
          </button>
        </section>

        {#if releasesLoading || releaseGroups.length > 0}
          <section class="block">
            <h2 class="section-title">Доступные загрузки</h2>
            <ReleaseList groups={releaseGroups} loading={releasesLoading} ondownload={downloadRelease} />
          </section>
        {/if}
      </div>

      <div class="overview-side">
        <Card>
          <div class="card-head">
            <h3 class="card-title">Достижения</h3>
            {#if gameAchievements}
              <button class="link" onclick={() => (tab = 'achievements')}>Показать все</button>
            {/if}
          </div>
          {#if gameAchievements}
            <div class="ach-summary">
              <div class="ach-badge">
                <Trophy size="2rem" strokeWidth={1.8} />
              </div>
              <div class="ach-count">
                <span class="ach-nums"><strong>{gameAchievements.earned}</strong> / {gameAchievements.total}</span>
                <span class="ach-label">Достижений получено</span>
              </div>
            </div>
            <ProgressBar value={gameAchievements.earned} max={gameAchievements.total} />
            <div class="ach-recent">
              <span class="ach-recent-label">Последнее достижение</span>
              {#each gameAchievements.recent.slice(0, 1) as a (a.name)}
                <div class="ach-item">
                  <div class="ach-icon">
                    <Trophy size="1.6rem" strokeWidth={1.8} />
                  </div>
                  <div class="ach-text">
                    <span class="ach-name">{a.name}</span>
                    <span class="ach-desc">{a.description}</span>
                  </div>
                  <span class="ach-date">{a.date}</span>
                </div>
              {/each}
            </div>
          {:else}
            <p class="muted">Пока нет полученных достижений.</p>
          {/if}
        </Card>

        <Card>
          <div class="card-head">
            <h3 class="card-title">Установленные дополнения</h3>
            {#if gameDlcs.length > 0}
              <span class="muted small">
                {gameDlcs.filter((d) => d.installed).length} / {gameDlcs.length}
              </span>
            {/if}
          </div>
          {#if gameDlcs.length === 0}
            <p class="muted">У этой игры нет дополнений.</p>
          {:else}
            <div class="dlc-list">
              {#each gameDlcs as dlc (dlc.id)}
                <div class="dlc">
                  <div class="dlc-text">
                    <span class="dlc-name">{dlc.name}</span>
                    <span class="dlc-kind">{dlc.kind}</span>
                  </div>
                  {#if dlc.installed}
                    <span class="dlc-state installed">
                      Установлено
                      <CircleCheck size="1.5rem" strokeWidth={1.8} />
                    </span>
                  {:else}
                    <span class="dlc-state">Не установлено</span>
                  {/if}
                </div>
              {/each}
            </div>
            <button class="link with-icon" onclick={() => (tab = 'addons')}>
              Управление дополнениями
              <ExternalLink size="1.4rem" strokeWidth={1.8} />
            </button>
          {/if}
        </Card>
      </div>
    </div>

    <section class="block">
      <h2 class="section-title">Быстрые действия</h2>
      <div class="quick-actions">
        {#each quickActions as action (action.id)}
          <button
            class="qa"
            class:danger={action.danger}
            title={action.sub}
            onclick={() => quickAction(action.id)}
          >
            <action.icon size="1.6rem" strokeWidth={1.8} />
            <span class="qa-label">{action.label}</span>
          </button>
        {/each}
      </div>
    </section>
  {:else if tab === 'addons'}
    {#if gameDlcs.length === 0}
      <EmptyState title="Дополнений нет" description="У этой игры пока нет доступных дополнений.">
        {#snippet icon()}
          <Gamepad2 size="2.2rem" strokeWidth={1.8} />
        {/snippet}
      </EmptyState>
    {:else}
      <div class="addons">
        {#each gameDlcs as dlc (dlc.id)}
          <div class="addon">
            <div class="dlc-text">
              <span class="dlc-name">{dlc.name}</span>
              <span class="dlc-kind">{dlc.kind}</span>
            </div>
            {#if dlc.installed}
              <StatusBadge kind="success" label="Установлено" dot={false} />
            {:else}
              <Button size="sm" onclick={() => toast(`«${dlc.name}» добавлено в очередь`)}>
                <Download size="1.4rem" strokeWidth={1.8} />
                Установить
              </Button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {:else if tab === 'achievements'}
    {#if gameAchievements}
      <section class="block">
        <div class="ach-summary">
          <div class="ach-badge">
            <Trophy size="2rem" strokeWidth={1.8} />
          </div>
          <div class="ach-count">
            <span class="ach-nums"><strong>{gameAchievements.earned}</strong> / {gameAchievements.total}</span>
            <span class="ach-label">Достижений получено</span>
          </div>
        </div>
        <ProgressBar value={gameAchievements.earned} max={gameAchievements.total} />
        <div class="ach-list">
          {#each gameAchievements.recent as a (a.name)}
            <div class="ach-item">
              <div class="ach-icon">
                <Trophy size="1.6rem" strokeWidth={1.8} />
              </div>
              <div class="ach-text">
                <span class="ach-name">{a.name}</span>
                <span class="ach-desc">{a.description}</span>
              </div>
              <span class="ach-date">{a.date}</span>
            </div>
          {/each}
        </div>
      </section>
    {:else}
      <EmptyState title="Достижений пока нет" description="Запустите игру, чтобы начать получать достижения.">
        {#snippet icon()}
          <Trophy size="2.2rem" strokeWidth={1.8} />
        {/snippet}
      </EmptyState>
    {/if}
  {:else if tab === 'screenshots'}
    <EmptyState title="Скриншотов пока нет" description="Скриншоты, сделанные через оверлей Typhon, появятся здесь.">
      {#snippet icon()}
        <Image size="2.2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button onclick={() => toast('Папка скриншотов недоступна в demo')}>Открыть папку скриншотов</Button>
      {/snippet}
    </EmptyState>
  {:else if tab === 'notes'}
    <EmptyState title="Заметок пока нет" description="Храните здесь пароли от сейфов, билды и прочие игровые записи.">
      {#snippet icon()}
        <NotebookPen size="2.2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button onclick={() => toast('Заметки недоступны в demo')}>
          <SquarePen size="1.5rem" strokeWidth={1.8} />
          Добавить заметку
        </Button>
      {/snippet}
    </EmptyState>
  {/if}

  <AddDownloadModal bind:open={downloadModalOpen} initialSource={downloadSource} origin={downloadOrigin} />
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

  .title {
    font-size: var(--font-hero);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.05;
    text-shadow: 0 1px 0.6rem rgba(0, 0, 0, 0.5);
  }

  .genres {
    display: block;
    margin-top: var(--space-3);
    font-size: var(--font-sm);
    color: rgba(255, 255, 255, 0.75);
    text-shadow: 0 1px 0.6rem rgba(0, 0, 0, 0.5);
  }

  .meta {
    display: flex;
    gap: var(--space-6);
    margin-top: var(--space-4);
    flex-wrap: wrap;
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
    gap: 0.7rem;
    font-size: var(--font-sm);
    color: rgba(255, 255, 255, 0.85);
    text-shadow: 0 1px 0.6rem rgba(0, 0, 0, 0.5);
  }

  .meta-item :global(svg) {
    width: 1.5rem;
    height: 1.5rem;
    color: var(--text-2);
  }

  .meta-label {
    color: var(--text-2);
  }

  .tagline {
    margin-top: var(--space-4);
    max-width: 52rem;
    font-size: var(--font-md);
    color: rgba(255, 255, 255, 0.8);
    text-shadow: 0 1px 0.6rem rgba(0, 0, 0, 0.5);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-5);
    flex-wrap: wrap;
  }

  .installed-group {
    display: inline-flex;
  }

  .installed-group :global(button:first-child) {
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
    border-right: none;
  }

  .installed-group :global(button:last-child) {
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
    padding: 0 1rem;
  }

  .size-hint {
    font-size: var(--font-sm);
    color: rgba(255, 255, 255, 0.75);
    text-shadow: 0 1px 0.6rem rgba(0, 0, 0, 0.5);
  }

  .tabs-row {
    margin: var(--space-6) 0 var(--space-8);
  }

  .overview {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 38rem;
    gap: var(--space-12);
    align-items: start;
    margin-bottom: var(--space-10);
  }

  .overview-main {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
  }

  .overview-side {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
  }

  .block {
    margin-bottom: var(--space-8);
  }

  .overview-main .block {
    margin-bottom: 0;
  }

  .section-title {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  .card-title {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .description {
    font-size: var(--font-md);
    line-height: 1.65;
    color: var(--text-2);
    max-width: var(--prose-max);
  }

  .props {
    display: flex;
    flex-direction: column;
    margin-top: var(--space-6);
    max-width: var(--prose-max);
  }

  .props .prop {
    display: grid;
    grid-template-columns: 18rem 1fr;
    gap: var(--space-4);
    padding: 1.1rem 0;
  }

  .props .prop + .prop {
    border-top: 1px solid var(--border);
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

  .expand {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: var(--space-4);
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--accent-text);
    border-radius: var(--radius-sm);
    transition: color var(--dur) var(--ease);
  }

  .expand:hover {
    color: var(--accent-hover);
  }

  .expand :global(svg) {
    transition: transform var(--dur) var(--ease);
  }

  .link {
    font-size: var(--font-sm);
    color: var(--accent-text);
    border-radius: var(--radius-sm);
    transition: color var(--dur) var(--ease);
  }

  .link:hover {
    color: var(--accent-hover);
  }

  .link.with-icon {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    margin-top: var(--space-4);
  }

  .ach-summary {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin: var(--space-4) 0;
  }

  .ach-badge {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4rem;
    height: 4rem;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--text-2);
    flex-shrink: 0;
  }

  .ach-count {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .ach-nums {
    font-size: var(--font-lg);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .ach-nums strong {
    font-size: 2.2rem;
    font-weight: 600;
    color: var(--text);
  }

  .ach-label {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .ach-recent {
    margin-top: var(--space-4);
  }

  .ach-recent-label {
    display: block;
    font-size: var(--font-xs);
    color: var(--text-3);
    margin-bottom: 1rem;
  }

  .ach-list {
    display: flex;
    flex-direction: column;
    margin-top: var(--space-5);
  }

  .ach-list .ach-item + .ach-item {
    border-top: 1px solid var(--border);
  }

  .ach-item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0.9rem 0;
  }

  .ach-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.6rem;
    height: 3.6rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    color: var(--text-2);
    flex-shrink: 0;
  }

  .ach-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .ach-name {
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .ach-desc {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .ach-date {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
    margin-top: var(--space-3);
  }

  .muted.small {
    font-size: var(--font-xs);
    margin: 0;
  }

  .dlc-list {
    display: flex;
    flex-direction: column;
    margin-top: var(--space-2);
  }

  .dlc {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: 1rem 0;
  }

  .dlc + .dlc {
    border-top: 1px solid var(--border);
  }

  .dlc-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .dlc-name {
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dlc-kind {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .dlc-state {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  .dlc-state.installed :global(svg) {
    color: var(--success);
  }

  .addons {
    display: flex;
    flex-direction: column;
    max-width: 86rem;
  }

  .addon {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 1.2rem 0;
  }

  .addon + .addon {
    border-top: 1px solid var(--border);
  }

  .quick-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .qa {
    display: inline-flex;
    align-items: center;
    gap: 0.8rem;
    height: var(--control-md);
    padding: 0 1.2rem;
    border-radius: var(--radius-md);
    color: var(--text-2);
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .qa:hover {
    background: var(--hover-strong);
    color: var(--text);
  }

  .qa :global(svg) {
    flex-shrink: 0;
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  .qa:hover :global(svg) {
    color: var(--text-2);
  }

  .qa.danger:hover {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .qa.danger:hover :global(svg) {
    color: var(--danger);
  }

  .qa-label {
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
  }

  @media (max-width: 1400px) {
    .overview {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--space-8);
    }
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
