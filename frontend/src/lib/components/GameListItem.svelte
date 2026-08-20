<script lang="ts">
  import { CircleCheck, EllipsisVertical, Play } from '@lucide/svelte';
  import type { Game } from '../mock/types';
  import { navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { gb } from '../utils/format';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import IconButton from './IconButton.svelte';

  let { game }: { game: Game } = $props();

  const menuItems = [
    { id: 'settings', label: 'Параметры игры' },
    { id: 'verify', label: 'Проверить файлы' },
    { id: 'folder', label: 'Открыть папку' },
    { id: 'uninstall', label: 'Удалить игру', danger: true, separator: true },
  ];

  function onMenu(id: string) {
    if (id === 'uninstall') toast(`«${game.title}» — удаление недоступно в demo`, 'danger');
    else toast('Действие недоступно в demo');
  }
</script>

<div class="row">
  <button class="game" onclick={() => navigate('game', { id: game.id })}>
    <div class="thumb">
      <Artwork src={game.cover} alt={game.title} radius="var(--radius-sm)" />
    </div>
    <div class="titles">
      <span class="title">{game.title}</span>
      <span class="tags">
        {#each game.genres.slice(0, 3) as genre (genre)}
          <span class="tag">{genre}</span>
        {/each}
      </span>
    </div>
  </button>
  <span class="cell version">{game.version}</span>
  <span class="cell size">{gb(game.sizeGb)}</span>
  <span class="cell last">{game.lastPlayed ?? '—'}</span>
  <span class="cell state">
    <CircleCheck size={16} strokeWidth={1.8} />
    Установлено
  </span>
  <div class="actions">
    <Button variant="primary" size="sm" onclick={() => toast(`Запуск «${game.title}»...`)}>
      <Play size={14} strokeWidth={2} fill="currentColor" />
      Играть
    </Button>
    <DropdownMenu items={menuItems} onselect={onMenu}>
      {#snippet trigger({ toggle })}
        <IconButton label="Меню" size="sm" onclick={toggle}>
          <EllipsisVertical size={16} strokeWidth={1.8} />
        </IconButton>
      {/snippet}
    </DropdownMenu>
  </div>
</div>

<style>
  .row {
    display: grid;
    grid-template-columns: minmax(240px, 1fr) 120px 90px 130px 140px auto;
    align-items: center;
    gap: var(--space-4);
    padding: 10px var(--space-4);
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
    width: 44px;
    height: 58px;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .titles {
    display: flex;
    flex-direction: column;
    gap: 5px;
    min-width: 0;
  }

  .title {
    font-size: 14.5px;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tags {
    display: flex;
    gap: 5px;
  }

  .tag {
    padding: 2px 8px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: rgba(255, 255, 255, 0.03);
    font-size: 11.5px;
    color: var(--text-3);
    white-space: nowrap;
  }

  .cell {
    font-size: 13.5px;
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .state {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    color: var(--text-2);
  }

  .state :global(svg) {
    color: var(--success);
    flex-shrink: 0;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 6px;
    justify-self: end;
  }

  @media (max-width: 1240px) {
    .row {
      grid-template-columns: minmax(200px, 1fr) 100px 80px 140px auto;
    }

    .last {
      display: none;
    }
  }
</style>
