<script lang="ts">
  import { CircleCheck, HardDrive, LayoutGrid, List, RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import GameListItem from '../../lib/components/GameListItem.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { games } from '../../lib/mock/games';
  import { openFolder } from '../../lib/services/settings';
  import { settings } from '../../lib/stores/settings';
  import { storageInfo } from '../../lib/stores/storage';
  import { toast } from '../../lib/stores/toasts';
  import { installedView } from '../../lib/stores/ui';
  import { bytesLabel, gb, plural } from '../../lib/utils/format';

  const installed = games.filter((g) => g.installed);
  const usedPct = $derived($storageInfo ? ($storageInfo.usedBytes / $storageInfo.totalBytes) * 100 : 0);

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
  subtitle="{installed.length} {plural(installed.length, 'игра', 'игры', 'игр')} · {gb(
    installed.reduce((sum, g) => sum + g.sizeGb, 0),
  )}"
>
  {#snippet actions()}
    <Button onclick={() => toast('Все игры обновлены', 'success')}>
      <RefreshCw size="1.6rem" strokeWidth={1.8} />
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
          <List size="1.6rem" strokeWidth={1.8} />
        {:else}
          <LayoutGrid size="1.6rem" strokeWidth={1.8} />
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
      <Button onclick={openGamesFolder}>Открыть папку игр</Button>
    </div>
  </section>
{/if}

<style>
  .table {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-2);
  }

  .thead {
    display: grid;
    grid-template-columns: minmax(24rem, 1fr) 12rem 9rem 13rem 14rem auto;
    gap: var(--space-4);
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

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(18.5rem, 1fr));
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

  @media (max-width: 1240px) {
    .thead {
      grid-template-columns: minmax(20rem, 1fr) 10rem 8rem 14rem auto;
    }

    .thead .last {
      display: none;
    }

    .storage-row {
      flex-wrap: wrap;
    }
  }
</style>
