<script lang="ts">
  import { CircleCheck, HardDrive, LayoutGrid, List, RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import GameListItem from '../../lib/components/GameListItem.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { games } from '../../lib/mock/games';
  import { storage } from '../../lib/mock/user';
  import { toast } from '../../lib/stores/toasts';
  import { installedView } from '../../lib/stores/ui';
  import { gb, plural } from '../../lib/utils/format';

  const installed = games.filter((g) => g.installed);
  const usedPct = (storage.usedGb / storage.totalGb) * 100;
</script>

<PageHeader
  title="Установлено"
  subtitle="{installed.length} {plural(installed.length, 'игра', 'игры', 'игр')} · {gb(
    installed.reduce((sum, g) => sum + g.sizeGb, 0),
  )}"
>
  {#snippet actions()}
    <Button onclick={() => toast('Все игры обновлены', 'success')}>
      <RefreshCw size={16} strokeWidth={1.8} />
      Проверить обновления
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
          <List size={16} strokeWidth={1.8} />
        {:else}
          <LayoutGrid size={16} strokeWidth={1.8} />
        {/if}
      {/snippet}
    </SegmentedControl>
  {/snippet}
</PageHeader>

{#if $installedView === 'list'}
  <div class="table">
    <div class="thead">
      <span>Игра</span>
      <span>Версия</span>
      <span>Размер</span>
      <span class="last">Последний запуск</span>
      <span>Состояние</span>
      <span></span>
    </div>
    {#each installed as game (game.id)}
      <GameListItem {game} />
    {/each}
  </div>
{:else}
  <div class="grid">
    {#each installed as game (game.id)}
      <GameCard {game} meta={gb(game.sizeGb)} />
    {/each}
  </div>
{/if}

<section class="storage-card">
  <h3>Хранилище игр</h3>
  <div class="storage-row">
    <div class="disk">
      <div class="disk-icon">
        <HardDrive size={20} strokeWidth={1.8} />
        <CircleCheck size={14} strokeWidth={2} class="disk-ok" />
      </div>
      <div class="disk-text">
        <span class="disk-name">{storage.disk}</span>
        <span class="disk-meta">1 ТБ · {storage.fs}</span>
      </div>
    </div>
    <div class="capacity">
      <div class="capacity-bar">
        <div class="capacity-fill" style:width="{usedPct}%"></div>
      </div>
      <div class="capacity-legend">
        <span class="legend-item"><span class="legend-dot used"></span>Занято {storage.usedGb} ГБ ({Math.round(usedPct)}%)</span>
        <span class="legend-item">
          <span class="legend-dot free"></span>Свободно {storage.totalGb - storage.usedGb} ГБ ({Math.round(100 - usedPct)}%)
        </span>
      </div>
    </div>
    <Button onclick={() => toast('Управление хранилищем недоступно в demo')}>Управление хранилищем</Button>
  </div>
</section>

<style>
  .table {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-2);
  }

  .thead {
    display: grid;
    grid-template-columns: minmax(240px, 1fr) 120px 90px 130px 140px auto;
    gap: var(--space-4);
    padding: 10px var(--space-4) 12px;
    border-bottom: 1px solid var(--border);
    margin-bottom: 4px;
  }

  .thead span {
    font-size: 12.5px;
    font-weight: 500;
    color: var(--text-3);
  }

  .thead span:last-child {
    justify-self: end;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(158px, 1fr));
    gap: var(--space-5);
  }

  .storage-card {
    margin-top: var(--space-6);
    padding: var(--space-5) var(--space-6);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .storage-card h3 {
    font-size: 15px;
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
    width: 44px;
    height: 44px;
    border-radius: var(--radius-md);
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-2);
  }

  .disk-icon :global(.disk-ok) {
    position: absolute;
    right: -4px;
    top: -4px;
    color: var(--success);
    background: var(--surface);
    border-radius: 50%;
  }

  .disk-text {
    display: flex;
    flex-direction: column;
  }

  .disk-name {
    font-size: 14px;
    font-weight: 550;
  }

  .disk-meta {
    font-size: 12.5px;
    color: var(--text-3);
  }

  .capacity {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 9px;
  }

  .capacity-bar {
    height: 8px;
    border-radius: 99px;
    background: rgba(255, 255, 255, 0.07);
    overflow: hidden;
  }

  .capacity-fill {
    height: 100%;
    border-radius: 99px;
    background: var(--accent);
  }

  .capacity-legend {
    display: flex;
    gap: var(--space-5);
  }

  .legend-item {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-size: 12.5px;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .legend-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .legend-dot.used {
    background: var(--accent);
  }

  .legend-dot.free {
    background: rgba(255, 255, 255, 0.18);
  }

  @media (max-width: 1240px) {
    .thead {
      grid-template-columns: minmax(200px, 1fr) 100px 80px 140px auto;
    }

    .thead .last {
      display: none;
    }

    .storage-row {
      flex-wrap: wrap;
    }
  }
</style>
