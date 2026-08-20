<script lang="ts">
  import {
    Activity,
    ArrowDown,
    ArrowUp,
    ChevronDown,
    Clock,
    Download,
    GripVertical,
    Layers,
    Settings,
    Upload,
    X,
  } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DownloadItem from '../../lib/components/DownloadItem.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { gameById } from '../../lib/mock/games';
  import { active, moveInQueue, queue, removeFromQueue, stats } from '../../lib/stores/downloads';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { gb, speed } from '../../lib/utils/format';

  let speedLimit = $state('Без ограничений');

  const statCards = $derived([
    { label: 'Скорость загрузки', value: speed($stats.downSpeed), icon: ArrowDown, tint: 'accent' },
    { label: 'Скорость отдачи', value: speed($stats.upSpeed), icon: ArrowUp, tint: 'success' },
    { label: 'Активные загрузки', value: String($stats.activeCount), icon: Activity, tint: 'success' },
    { label: 'В очереди', value: String($queue.length), icon: Layers, tint: 'accent' },
  ]);
</script>

<PageHeader title="Загрузки">
  {#snippet actions()}
    <DropdownMenu
      items={[
        { id: 'none', label: 'Без ограничений' },
        { id: '10', label: '10 МБ/с' },
        { id: '25', label: '25 МБ/с' },
        { id: '50', label: '50 МБ/с' },
      ]}
      onselect={(id) => {
        speedLimit = id === 'none' ? 'Без ограничений' : `${id} МБ/с`;
        toast(`Ограничение скорости: ${speedLimit}`);
      }}
    >
      {#snippet trigger({ open, toggle })}
        <Button onclick={toggle}>
          <Clock size="1.6rem" strokeWidth={1.8} />
          Ограничение скорости
          <ChevronDown size="1.5rem" strokeWidth={1.8} />
        </Button>
      {/snippet}
    </DropdownMenu>
    <Button onclick={() => navigate('settings')}>
      <Settings size="1.6rem" strokeWidth={1.8} />
      Настройки загрузок
    </Button>
  {/snippet}
</PageHeader>

<div class="stats">
  {#each statCards as stat (stat.label)}
    <div class="stat-card">
      <div class="stat-icon {stat.tint}">
        <stat.icon size="2rem" strokeWidth={1.8} />
      </div>
      <div class="stat-text">
        <span class="stat-label">{stat.label}</span>
        <span class="stat-value">{stat.value}</span>
      </div>
    </div>
  {/each}
</div>

<section class="section">
  <h2>Активные загрузки ({$active.length})</h2>
  {#if $active.length === 0}
    <EmptyState title="Нет активных загрузок" description="Добавьте игру из библиотеки, чтобы начать загрузку.">
      {#snippet icon()}
        <Download size="2.2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button onclick={() => navigate('library')}>В библиотеку</Button>
      {/snippet}
    </EmptyState>
  {:else}
    <div class="downloads">
      {#each $active as download (download.id)}
        <DownloadItem {download} />
      {/each}
    </div>
  {/if}
</section>

<section class="section">
  <h2>В очереди ({$queue.length})</h2>
  {#if $queue.length === 0}
    <p class="muted">Очередь загрузки пуста.</p>
  {:else}
    <div class="queue">
      {#each $queue as q, i (q.id)}
        {@const game = gameById(q.gameId)}
        <div class="queue-row">
          <span class="grip" title="Перетащите для изменения порядка">
            <GripVertical size="1.7rem" strokeWidth={1.8} />
          </span>
          <button class="queue-game" onclick={() => game && navigate('game', { id: game.id })}>
            <div class="queue-thumb">
              <Artwork src={game?.cover ?? ''} alt={game?.title ?? ''} radius="0.6rem" />
            </div>
            <span class="queue-title">{game?.title}</span>
          </button>
          <span class="queue-size">{gb(q.sizeGb)}</span>
          <span class="queue-state">В очереди</span>
          <div class="queue-actions">
            <IconButton label="Выше" size="sm" onclick={() => moveInQueue(q.id, -1)}>
              <span class="dim" class:disabled={i === 0}><ArrowUp size="1.6rem" strokeWidth={1.8} /></span>
            </IconButton>
            <IconButton label="Ниже" size="sm" onclick={() => moveInQueue(q.id, 1)}>
              <span class="dim" class:disabled={i === $queue.length - 1}>
                <ArrowDown size="1.6rem" strokeWidth={1.8} />
              </span>
            </IconButton>
            <IconButton label="Убрать из очереди" size="sm" onclick={() => removeFromQueue(q.id)}>
              <X size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</section>

<style>
  .stats {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: var(--space-4);
    margin-bottom: var(--space-8);
  }

  .stat-card {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-5);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .stat-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4.6rem;
    height: 4.6rem;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .stat-icon.accent {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .stat-icon.success {
    background: var(--success-subtle);
    color: var(--success);
  }

  .stat-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .stat-label {
    font-size: 1.2rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
    white-space: nowrap;
  }

  .stat-value {
    font-size: 2.2rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    line-height: 1.2;
  }

  .section {
    margin-bottom: var(--space-8);
  }

  .section h2 {
    font-size: 1.8rem;
    margin-bottom: var(--space-4);
  }

  .downloads {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .muted {
    font-size: 1.4rem;
    color: var(--text-3);
  }

  .queue {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-2);
  }

  .queue-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0.8rem 1rem;
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .queue-row:hover {
    background: rgba(255, 255, 255, 0.025);
  }

  .queue-row + .queue-row {
    border-top: 1px solid var(--border);
  }

  .grip {
    color: var(--text-3);
    cursor: grab;
    flex-shrink: 0;
    opacity: 0.6;
  }

  .queue-row:hover .grip {
    opacity: 1;
  }

  .queue-game {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex: 1;
    min-width: 0;
    text-align: left;
  }

  .queue-thumb {
    width: 4rem;
    height: 5.2rem;
    flex-shrink: 0;
    border-radius: 0.6rem;
    overflow: hidden;
  }

  .queue-title {
    font-size: 1.5rem;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .queue-size {
    font-size: 1.4rem;
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    min-width: 7rem;
    text-align: right;
  }

  .queue-state {
    font-size: 1.4rem;
    color: var(--text-3);
    min-width: 9rem;
    text-align: right;
  }

  .queue-actions {
    display: flex;
    gap: 2px;
    margin-left: var(--space-3);
  }

  .dim {
    display: inline-flex;
  }

  .dim.disabled {
    opacity: 0.3;
  }

  @media (max-width: 1240px) {
    .stats {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>
