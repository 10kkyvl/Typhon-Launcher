<script lang="ts">
  import {
    EllipsisVertical,
    FolderOpen,
    Gamepad2,
    HardDrive,
    LayoutGrid,
    List,
    Play,
    Plus,
    RefreshCw,
    Square,
  } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { inWails } from '../../lib/services/backend';
  import {
    addGame,
    playGame,
    selectExecutable,
    setExecutable,
    stopGame,
    type LibraryGame,
  } from '../../lib/services/library';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import { openFolder } from '../../lib/services/settings';
  import { rescan, scanProgress, scanSummary, scanning } from '../../lib/stores/discovery';
  import { installedGames, runningGames } from '../../lib/stores/library';
  import { gameArt, loadArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { settings } from '../../lib/stores/settings';
  import { storageInfo } from '../../lib/stores/storage';
  import { toast } from '../../lib/stores/toasts';
  import { installedView } from '../../lib/stores/ui';
  import { updatesByGame } from '../../lib/stores/updates';
  import { bytesLabel, playtime, plural, relativeDate } from '../../lib/utils/format';

  const totalBytes = $derived($installedGames.reduce((sum, g) => sum + g.sizeBytes, 0));
  const usedPct = $derived($storageInfo ? ($storageInfo.usedBytes / $storageInfo.totalBytes) * 100 : 0);

  function coverFor(game: LibraryGame) {
    return (game.canonicalGameId && $gameArt[game.canonicalGameId]?.cover) || game.cover;
  }

  function heroFor(game: LibraryGame) {
    return (game.canonicalGameId && $gameArt[game.canonicalGameId]?.hero) || coverFor(game);
  }

  $effect(() => {
    loadArt($installedGames.map((game) => game.canonicalGameId).filter((cid): cid is string => Boolean(cid)));
  });

  let addOpen = $state(false);
  let newExecutable = $state('');
  let newTitle = $state('');
  let adding = $state(false);

  function titleFromPath(path: string) {
    const base = path.split(/[\\/]/).pop() ?? '';
    return base.replace(/\.exe$/i, '').replace(/[_.-]+/g, ' ').trim();
  }

  function openAddDialog() {
    if (!inWails) {
      toast('Добавление игр доступно только в desktop-сборке');
      return;
    }
    newExecutable = '';
    newTitle = '';
    addOpen = true;
  }

  async function browseExecutable() {
    try {
      const path = await selectExecutable('Выберите исполняемый файл игры');
      if (path) {
        newExecutable = path;
        if (!newTitle.trim()) newTitle = titleFromPath(path);
      }
    } catch {
      toast('Не удалось открыть диалог выбора файла', 'danger');
    }
  }

  async function submitAdd() {
    if (!newExecutable.trim()) return;
    adding = true;
    try {
      const game = await addGame(newExecutable.trim(), newTitle.trim());
      toast(`«${game.title}» добавлена в библиотеку`, 'success');
      addOpen = false;
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось добавить игру', 'danger');
    } finally {
      adding = false;
    }
  }

  async function play(game: LibraryGame) {
    try {
      await playGame(game.id);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось запустить игру', 'danger');
    }
  }

  async function stop(game: LibraryGame) {
    try {
      await stopGame(game.id);
    } catch {
      toast('Не удалось остановить игру', 'danger');
    }
  }

  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('disk');
  let removeTarget = $state<LibraryGame | null>(null);

  async function onMenu(game: LibraryGame, action: string) {
    if (action === 'folder') {
      try {
        await openFolder(game.installDir);
      } catch {
        toast('Папка недоступна', 'danger');
      }
    } else if (action === 'executable') {
      await chooseExecutable(game);
    } else if (action === 'uninstall' || action === 'remove') {
      removeMode = action === 'uninstall' ? 'disk' : 'library';
      removeTarget = game;
      removeOpen = true;
    }
  }

  const menuItems = [
    { id: 'folder', label: 'Открыть папку' },
    { id: 'executable', label: 'Выбрать файл запуска' },
    { id: 'uninstall', label: 'Удалить с компьютера', danger: true, separator: true },
    { id: 'remove', label: 'Удалить из библиотеки', danger: true },
  ];

  const scanLabel = $derived(
    $scanProgress.total > 0
      ? `Поиск ${$scanProgress.processed}/${$scanProgress.total}`
      : 'Поиск игр...',
  );

  async function findGames() {
    if (!inWails) {
      toast('Поиск игр доступен только в desktop-сборке');
      return;
    }
    if ($scanning) return;
    try {
      const result = await rescan();
      if (result.cancelled) {
        toast('Поиск остановлен');
        return;
      }
      toast(scanSummary(result), result.errors > 0 ? 'danger' : 'success');
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось выполнить поиск', 'danger');
    }
  }

  async function chooseExecutable(game: LibraryGame) {
    try {
      const path = await selectExecutable(`Файл запуска — ${game.title}`);
      if (!path) return;
      await setExecutable(game.id, path);
      toast(`Файл запуска для «${game.title}» сохранён`, 'success');
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось выбрать файл', 'danger');
    }
  }

  let librarySetupOpen = $state(false);

  async function openGamesFolder() {
    try {
      await openFolder($settings?.gamesPath ?? '');
    } catch {
      toast('Папка с играми недоступна', 'danger');
    }
  }
</script>

<PageHeader
  title="Установлено"
  subtitle={$installedGames.length > 0
    ? `${$installedGames.length} ${plural($installedGames.length, 'игра', 'игры', 'игр')} · ${bytesLabel(totalBytes)}`
    : 'Локальная библиотека пуста'}
>
  {#snippet actions()}
    <SegmentedControl
      bind:value={$installedView}
      options={[
        { id: 'list', label: 'Список' },
        { id: 'grid', label: 'Сетка' },
      ]}
    >
      {#snippet item(option)}
        {#if option.id === 'list'}
          <List size="1.6rem" strokeWidth={1.8} />
        {:else}
          <LayoutGrid size="1.6rem" strokeWidth={1.8} />
        {/if}
      {/snippet}
    </SegmentedControl>
    <Button onclick={findGames} disabled={$scanning}>
      <RefreshCw size="1.5rem" strokeWidth={2} class={$scanning ? 'spin' : ''} />
      {$scanning ? scanLabel : 'Найти игры'}
    </Button>
    <Button variant="primary" onclick={openAddDialog}>
      <Plus size="1.5rem" strokeWidth={2} />
      Добавить игру
    </Button>
  {/snippet}
</PageHeader>

{#if $installedGames.length === 0}
  <EmptyState
    title="Игры ещё не добавлены"
    description="Найдите игры в папке библиотеки или добавьте установленную игру вручную, указав её исполняемый файл."
  >
    {#snippet icon()}
      <Gamepad2 size="2rem" strokeWidth={1.8} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={findGames} disabled={$scanning}>
        <RefreshCw size="1.5rem" strokeWidth={2} class={$scanning ? 'spin' : ''} />
        {$scanning ? scanLabel : 'Найти игры'}
      </Button>
      <Button onclick={openAddDialog}>
        <Plus size="1.5rem" strokeWidth={2} />
        Добавить игру
      </Button>
    {/snippet}
  </EmptyState>
{:else if $installedView === 'list'}
  <div class="list">
    {#each $installedGames as game (game.id)}
      {@const running = $runningGames.has(game.id)}
      {@const update = $updatesByGame.get(game.id)?.availability}
      <div class="row" class:running>
        <button class="game" onclick={() => navigate('game', { id: game.id })}>
          <div class="thumb">
            <Artwork src={heroFor(game)} alt={game.title} radius="var(--radius-sm)" />
          </div>
          <div class="titles">
            <span class="title">{game.title}</span>
            <span class="meta">
              {#if game.version}<span class="version">{game.version}</span><span class="sep">·</span>{/if}
              <span>{bytesLabel(game.sizeBytes)}</span>
            </span>
          </div>
        </button>
        <div class="cell last">
          <span class="cell-value">{relativeDate(game.lastPlayed)}</span>
          <span class="cell-label">Последний запуск</span>
        </div>
        <div class="cell time">
          <span class="cell-value">{game.playtimeSeconds > 0 ? playtime(game.playtimeSeconds) : '—'}</span>
          <span class="cell-label">Наиграно</span>
        </div>
        <div class="cell state">
          {#if running}
            <StatusBadge kind="accent" label="Запущена" plain />
          {:else if !game.executable}
            <StatusBadge kind="warning" label="Нужен файл запуска" plain />
          {:else if update?.kind === 'update'}
            <StatusBadge kind="warning" label="Обновление" plain />
          {:else if update?.kind === 'new_release'}
            <StatusBadge kind="neutral" label="Новый релиз" plain />
          {:else}
            <StatusBadge kind="success" label="Установлено" plain />
          {/if}
        </div>
        <div class="actions">
          {#if running}
            <Button size="sm" onclick={() => stop(game)}>
              <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
              Стоп
            </Button>
          {:else if !game.executable}
            <Button size="sm" onclick={() => chooseExecutable(game)}>
              <FolderOpen size="1.3rem" strokeWidth={1.8} />
              Указать файл
            </Button>
          {:else}
            <Button variant="primary" size="sm" onclick={() => play(game)}>
              <Play size="1.3rem" strokeWidth={2} fill="currentColor" />
              Играть
            </Button>
          {/if}
          <DropdownMenu items={menuItems} onselect={(id) => onMenu(game, id)}>
            {#snippet trigger({ toggle })}
              <IconButton label="Меню" size="sm" onclick={toggle}>
                <EllipsisVertical size="1.6rem" strokeWidth={1.8} />
              </IconButton>
            {/snippet}
          </DropdownMenu>
        </div>
      </div>
    {/each}
  </div>
{:else}
  <div class="grid">
    {#each $installedGames as game (game.id)}
      {@const update = $updatesByGame.get(game.id)?.availability}
      <div class="card">
        <button class="card-cover" onclick={() => navigate('game', { id: game.id })} aria-label={game.title}>
          <Artwork src={coverFor(game)} alt={game.title} ratio="3 / 4" radius="var(--radius-md)" />
        </button>
        <div class="card-info">
          <span class="card-title">{game.title}</span>
          <span class="card-meta">
            {bytesLabel(game.sizeBytes)}
            {#if update?.available}
              <span class="card-update">{update.kind === 'update' ? 'Обновление' : 'Новый релиз'}</span>
            {/if}
          </span>
        </div>
      </div>
    {/each}
  </div>
{/if}

{#if !$settings?.libraryPath}
  <section class="storage">
    <div class="storage-head">
      <div class="disk">
        <HardDrive size="1.8rem" strokeWidth={1.8} />
        <div class="disk-text">
          <span class="disk-name">Библиотека не настроена</span>
          <span class="disk-meta">Выберите диск, на котором будут жить игры, загрузки и скриншоты</span>
        </div>
      </div>
      <Button size="sm" variant="primary" onclick={() => (librarySetupOpen = true)}>
        <FolderOpen size="1.5rem" strokeWidth={1.8} />
        Выбрать папку
      </Button>
    </div>
  </section>
{:else if $storageInfo}
  <section class="storage">
    <div class="storage-head">
      <div class="disk">
        <HardDrive size="1.8rem" strokeWidth={1.8} />
        <div class="disk-text">
          <span class="disk-name">Хранилище · Диск {$storageInfo.volume || '—'}</span>
          <span class="disk-meta">
            {bytesLabel($storageInfo.totalBytes)}{$storageInfo.filesystem ? ` · ${$storageInfo.filesystem}` : ''}
          </span>
        </div>
      </div>
      <Button size="sm" variant="ghost" onclick={openGamesFolder}>
        <FolderOpen size="1.5rem" strokeWidth={1.8} />
        Открыть папку игр
      </Button>
    </div>
    <div class="capacity">
      <ProgressBar value={usedPct} height={5} />
      <div class="capacity-legend">
        <span>Занято {bytesLabel($storageInfo.usedBytes)} ({Math.round(usedPct)}%)</span>
        <span>Свободно {bytesLabel($storageInfo.freeBytes)}</span>
      </div>
    </div>
  </section>
{/if}

<LibrarySetupModal bind:open={librarySetupOpen} />

{#if removeTarget}
  <RemoveGameModal
    bind:open={removeOpen}
    bind:mode={removeMode}
    gameId={removeTarget.id}
    title={removeTarget.title}
  />
{/if}

<Modal bind:open={addOpen} title="Добавить установленную игру">
  <div class="form">
    <label class="field">
      <span class="field-label">Исполняемый файл</span>
      <div class="field-row">
        <input class="input" type="text" placeholder="C:\Games\Game\game.exe" bind:value={newExecutable} />
        <Button onclick={browseExecutable}>Обзор</Button>
      </div>
    </label>
    <label class="field">
      <span class="field-label">Название</span>
      <input class="input" type="text" placeholder="Название игры" bind:value={newTitle} />
    </label>
    <p class="form-hint">Папка установки будет определена автоматически по расположению файла.</p>
  </div>
  {#snippet footer()}
    <Button onclick={() => (addOpen = false)}>Отмена</Button>
    <Button variant="primary" disabled={!newExecutable.trim() || adding} onclick={submitAdd}>
      {adding ? 'Добавление...' : 'Добавить'}
    </Button>
  {/snippet}
</Modal>

<style>
  :global(.spin) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: grid;
    grid-template-columns: minmax(28rem, 1fr) 15rem 11rem 13rem auto;
    align-items: center;
    gap: var(--space-5);
    min-height: 8rem;
    padding: 1rem 1.2rem;
    margin: 0 -1.2rem;
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row:hover {
    background: var(--hover);
  }

  .game {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-width: 0;
    text-align: left;
  }

  .thumb {
    width: 10.4rem;
    height: 5.8rem;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .titles {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .title {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .meta {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .version {
    max-width: 18rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .sep {
    opacity: 0.6;
  }

  .cell {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .cell-value {
    font-size: var(--font-sm);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell-label {
    font-size: 1.2rem;
    color: var(--text-3);
    white-space: nowrap;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    justify-self: end;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--space-6) var(--space-5);
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    min-width: 0;
  }

  .card-cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    transition: transform var(--dur) var(--ease);
  }

  .card-cover:hover {
    transform: scale(1.01);
  }

  .card-info {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .card-title {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    line-height: 1.3;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .card-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .card-update {
    margin-left: 0.8rem;
    color: var(--warning);
  }

  .storage {
    margin-top: var(--space-10);
    padding-top: var(--space-6);
    border-top: 1px solid var(--border);
    max-width: 96rem;
  }

  .storage-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .disk {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    color: var(--text-2);
  }

  .disk-text {
    display: flex;
    flex-direction: column;
  }

  .disk-name {
    font-size: var(--font-md);
    font-weight: 500;
    color: var(--text);
  }

  .disk-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .capacity {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .capacity-legend {
    display: flex;
    justify-content: space-between;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .field-label {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
  }

  .field-row {
    display: flex;
    gap: 0.8rem;
  }

  .form-hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  @media (min-width: 2200px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    }
  }

  @media (max-width: 1400px) {
    .row {
      grid-template-columns: minmax(22rem, 1fr) 12rem 13rem auto;
    }

    .last {
      display: none;
    }

    .grid {
      grid-template-columns: repeat(auto-fill, minmax(14.5rem, 1fr));
    }
  }
</style>
