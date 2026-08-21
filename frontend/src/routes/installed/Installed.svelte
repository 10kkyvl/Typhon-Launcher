<script lang="ts">
  import {
    CircleCheck,
    EllipsisVertical,
    FolderOpen,
    Gamepad2,
    HardDrive,
    LayoutGrid,
    List,
    Play,
    Plus,
    Square,
  } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { inWails } from '../../lib/services/backend';
  import {
    addGame,
    playGame,
    removeGame,
    selectExecutable,
    stopGame,
    type LibraryGame,
  } from '../../lib/services/library';
  import { openFolder } from '../../lib/services/settings';
  import { libraryGames, runningGames } from '../../lib/stores/library';
  import { navigate } from '../../lib/stores/router';
  import { settings } from '../../lib/stores/settings';
  import { storageInfo } from '../../lib/stores/storage';
  import { toast } from '../../lib/stores/toasts';
  import { installedView } from '../../lib/stores/ui';
  import { updatesByGame } from '../../lib/stores/updates';
  import { bytesLabel, playtime, plural, relativeDate } from '../../lib/utils/format';

  const totalBytes = $derived($libraryGames.reduce((sum, g) => sum + g.sizeBytes, 0));
  const usedPct = $derived($storageInfo ? ($storageInfo.usedBytes / $storageInfo.totalBytes) * 100 : 0);

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

  async function onMenu(game: LibraryGame, action: string) {
    if (action === 'folder') {
      try {
        await openFolder(game.installDir);
      } catch {
        toast('Папка недоступна', 'danger');
      }
    } else if (action === 'remove') {
      try {
        await removeGame(game.id);
        toast(`«${game.title}» удалена из библиотеки`);
      } catch {
        toast('Не удалось удалить игру', 'danger');
      }
    }
  }

  const menuItems = [
    { id: 'folder', label: 'Открыть папку' },
    { id: 'remove', label: 'Удалить из библиотеки', danger: true, separator: true },
  ];

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
  subtitle={$libraryGames.length > 0
    ? `${$libraryGames.length} ${plural($libraryGames.length, 'игра', 'игры', 'игр')} · ${bytesLabel(totalBytes)}`
    : 'Локальная библиотека пуста'}
>
  {#snippet actions()}
    <Button variant="primary" onclick={openAddDialog}>
      <Plus size="1.6rem" strokeWidth={2} />
      Добавить игру
    </Button>
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
  {/snippet}
</PageHeader>

{#if $libraryGames.length === 0}
  <div class="empty-wrap">
    <EmptyState
      title="Игры ещё не добавлены"
      description="Добавьте установленную игру, указав её исполняемый файл — Typhon будет отслеживать запуски и время в игре."
    >
      {#snippet icon()}
        <Gamepad2 size="2.2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={openAddDialog}>
          <Plus size="1.6rem" strokeWidth={2} />
          Добавить игру
        </Button>
      {/snippet}
    </EmptyState>
  </div>
{:else if $installedView === 'list'}
  <div class="table">
    <div class="thead">
      <span>Игра</span>
      <span>Размер</span>
      <span class="last">Последний запуск</span>
      <span>Наиграно</span>
      <span>Состояние</span>
      <span></span>
    </div>
    {#each $libraryGames as game (game.id)}
      {@const running = $runningGames.has(game.id)}
      <div class="row">
        <button class="game" onclick={() => navigate('game', { id: game.id })}>
          <div class="thumb">
            <Artwork src={game.cover} alt={game.title} radius="var(--radius-sm)" />
          </div>
          <div class="titles">
            <span class="title">{game.title}</span>
            <span class="path">{game.installDir}</span>
          </div>
        </button>
        <span class="cell">{bytesLabel(game.sizeBytes)}</span>
        <span class="cell last">{relativeDate(game.lastPlayed)}</span>
        <span class="cell">{playtime(game.playtimeSeconds)}</span>
        <span class="cell state">
          {#if running}
            <StatusBadge kind="accent" label="Запущена" />
          {:else if $updatesByGame.get(game.id)?.availability.kind === 'update'}
            <StatusBadge kind="warning" label="Обновление" />
          {:else if $updatesByGame.get(game.id)?.availability.kind === 'new_release'}
            <StatusBadge kind="neutral" label="Новый релиз" />
          {:else}
            <CircleCheck size="1.6rem" strokeWidth={1.8} />
            Установлено
          {/if}
        </span>
        <div class="actions">
          {#if running}
            <Button size="sm" onclick={() => stop(game)}>
              <Square size="1.3rem" strokeWidth={2} fill="currentColor" />
              Стоп
            </Button>
          {:else}
            <Button variant="primary" size="sm" onclick={() => play(game)}>
              <Play size="1.4rem" strokeWidth={2} fill="currentColor" />
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
    {#each $libraryGames as game (game.id)}
      <div class="card">
        <button class="card-cover" onclick={() => navigate('game', { id: game.id })} aria-label={game.title}>
          <Artwork src={game.cover} alt={game.title} ratio="3 / 4" radius="var(--radius-lg)" />
        </button>
        <div class="card-info">
          <span class="card-title">{game.title}</span>
          <span class="card-meta">
            {bytesLabel(game.sizeBytes)}
            {#if $updatesByGame.get(game.id)?.availability.available}
              <span class="card-update">
                {$updatesByGame.get(game.id)?.availability.kind === 'update' ? 'Обновление' : 'Новый релиз'}
              </span>
            {/if}
          </span>
        </div>
      </div>
    {/each}
  </div>
{/if}

{#if $storageInfo}
  <section class="storage-card">
    <h3>Хранилище игр</h3>
    <div class="storage-row">
      <div class="disk">
        <div class="disk-icon">
          <HardDrive size="2rem" strokeWidth={1.8} />
          <CircleCheck size="1.4rem" strokeWidth={2} class="disk-ok" />
        </div>
        <div class="disk-text">
          <span class="disk-name">Диск ({$storageInfo.volume || '—'})</span>
          <span class="disk-meta">
            {bytesLabel($storageInfo.totalBytes)}{$storageInfo.filesystem ? ` · ${$storageInfo.filesystem}` : ''}
          </span>
        </div>
      </div>
      <div class="capacity">
        <div class="capacity-bar">
          <div class="capacity-fill" style:width="{usedPct}%"></div>
        </div>
        <div class="capacity-legend">
          <span class="legend-item">
            <span class="legend-dot used"></span>Занято {bytesLabel($storageInfo.usedBytes)} ({Math.round(usedPct)}%)
          </span>
          <span class="legend-item">
            <span class="legend-dot free"></span>Свободно {bytesLabel($storageInfo.freeBytes)} ({Math.round(100 - usedPct)}%)
          </span>
        </div>
      </div>
      <div class="storage-actions">
        <Button onclick={openGamesFolder}>
          <FolderOpen size="1.6rem" strokeWidth={1.8} />
          Открыть папку игр
        </Button>
      </div>
    </div>
  </section>
{/if}

<Modal bind:open={addOpen} title="Добавить установленную игру">
  <div class="form">
    <label class="field">
      <span class="field-label">Исполняемый файл</span>
      <div class="field-row">
        <input type="text" placeholder="C:\Games\Game\game.exe" bind:value={newExecutable} />
        <Button size="sm" onclick={browseExecutable}>Обзор</Button>
      </div>
    </label>
    <label class="field">
      <span class="field-label">Название</span>
      <input type="text" placeholder="Название игры" bind:value={newTitle} />
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
  .card-update {
    margin-left: 0.8rem;
    color: var(--accent);
  }

  .empty-wrap {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .table {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-2);
  }

  .thead,
  .row {
    display: grid;
    grid-template-columns: minmax(26rem, 1fr) 10rem 14rem 10rem 15rem auto;
    align-items: center;
    gap: var(--space-4);
  }

  .thead {
    padding: 1rem var(--space-4) 1.2rem;
    border-bottom: 1px solid var(--border);
    margin-bottom: 0.4rem;
  }

  .thead span {
    font-size: 1.3rem;
    font-weight: 500;
    color: var(--text-3);
  }

  .thead span:last-child {
    justify-self: end;
  }

  .row {
    padding: 1rem var(--space-4);
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: rgba(255, 255, 255, 0.025);
  }

  .game {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-width: 0;
    text-align: left;
  }

  .thumb {
    width: 4.4rem;
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
    font-size: 1.5rem;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .path {
    font-size: 1.3rem;
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell {
    font-size: 1.4rem;
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .state {
    display: inline-flex;
    align-items: center;
    gap: 0.7rem;
    overflow: visible;
  }

  .state :global(svg) {
    color: var(--success);
    flex-shrink: 0;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    justify-self: end;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(18.5rem, 1fr));
    gap: var(--space-5);
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    min-width: 0;
  }

  .card-cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-lg);
    overflow: hidden;
    border: 1px solid var(--border);
    transition:
      transform var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .card-cover:hover {
    transform: translateY(-1px);
    border-color: var(--border-strong);
  }

  .card-info {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .card-title {
    font-size: 1.5rem;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .card-meta {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .storage-card {
    margin-top: var(--space-6);
    padding: var(--space-5) var(--space-6);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .storage-card h3 {
    font-size: 1.6rem;
    margin-bottom: var(--space-4);
  }

  .storage-row {
    display: flex;
    align-items: center;
    gap: var(--space-6);
  }

  .disk {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-shrink: 0;
  }

  .disk-icon {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4.4rem;
    height: 4.4rem;
    border-radius: var(--radius-md);
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-2);
  }

  .disk-icon :global(.disk-ok) {
    position: absolute;
    right: -0.4rem;
    top: -0.4rem;
    color: var(--success);
    background: var(--surface);
    border-radius: 50%;
  }

  .disk-text {
    display: flex;
    flex-direction: column;
  }

  .disk-name {
    font-size: 1.5rem;
    font-weight: 550;
  }

  .disk-meta {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .capacity {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
  }

  .capacity-bar {
    height: 0.8rem;
    border-radius: 9.9rem;
    background: rgba(255, 255, 255, 0.07);
    overflow: hidden;
  }

  .capacity-fill {
    height: 100%;
    border-radius: 9.9rem;
    background: var(--accent);
  }

  .capacity-legend {
    display: flex;
    gap: var(--space-5);
  }

  .legend-item {
    display: inline-flex;
    align-items: center;
    gap: 0.7rem;
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .legend-dot {
    width: 0.8rem;
    height: 0.8rem;
    border-radius: 50%;
  }

  .legend-dot.used {
    background: var(--accent);
  }

  .legend-dot.free {
    background: rgba(255, 255, 255, 0.18);
  }

  .storage-actions {
    flex-shrink: 0;
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .field-label {
    font-size: 1.4rem;
    font-weight: 500;
    color: var(--text-2);
  }

  .field-row {
    display: flex;
    gap: 0.8rem;
  }

  .field input {
    flex: 1;
    min-width: 0;
    height: var(--control-md);
    padding: 0 1.2rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: var(--font-md);
    color: var(--text);
    outline: none;
    transition:
      border-color var(--dur) var(--ease),
      background var(--dur) var(--ease);
  }

  .field input:hover {
    border-color: var(--border-strong);
  }

  .field input:focus {
    border-color: rgba(104, 117, 232, 0.55);
    background: var(--surface-3);
  }

  .field input::placeholder {
    color: var(--text-3);
  }

  .form-hint {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  @media (max-width: 1240px) {
    .thead,
    .row {
      grid-template-columns: minmax(20rem, 1fr) 9rem 10rem 15rem auto;
    }

    .last {
      display: none;
    }

    .storage-row {
      flex-wrap: wrap;
    }
  }
</style>
