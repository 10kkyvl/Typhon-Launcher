<script lang="ts">
  import {
    CircleAlert,
    CircleCheck,
    Database,
    EllipsisVertical,
    Globe,
    Plus,
    RefreshCw,
    TriangleAlert,
  } from '@lucide/svelte';
  import AddSourceModal from '../../lib/components/AddSourceModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SourceDetailsModal from '../../lib/components/SourceDetailsModal.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { refresh, refreshAll, refreshingAll, remove, sources, toggle } from '../../lib/stores/sources';
  import type { Source, SourceHealth, SourceStatus } from '../../lib/services/sources';
  import { formatCount, relativeDate, truncateMiddle } from '../../lib/utils/format';

  let addOpen = $state(false);
  let detailsOpen = $state(false);
  let detailsId = $state<string | null>(null);

  const statusBadge: Record<SourceStatus, { kind: 'success' | 'neutral' | 'danger'; label: string }> = {
    active: { kind: 'success', label: 'Активен' },
    disabled: { kind: 'neutral', label: 'Отключен' },
    error: { kind: 'danger', label: 'Ошибка' },
    updating: { kind: 'neutral', label: 'Обновление' },
  };

  const healthMeta: Record<SourceHealth, { icon: typeof CircleCheck; color: string; label: string }> = {
    healthy: { icon: CircleCheck, color: 'var(--success)', label: 'Всё в порядке' },
    warning: { icon: TriangleAlert, color: 'var(--warning)', label: 'Есть предупреждения' },
    error: { icon: CircleAlert, color: 'var(--danger)', label: 'Ошибка источника' },
  };

  function openDetails(id: string) {
    detailsId = id;
    detailsOpen = true;
  }

  function sourceMenu(source: Source) {
    return [
      { id: 'refresh', label: source.status === 'updating' ? 'Обновление…' : 'Обновить сейчас' },
      { id: 'toggle', label: source.enabled ? 'Отключить' : 'Включить' },
      { id: 'details', label: 'Подробнее' },
      { id: 'remove', label: 'Удалить источник', danger: true, separator: true },
    ];
  }

  async function onSourceMenu(source: Source, action: string) {
    if (action === 'refresh') {
      if (source.status === 'updating') return;
      await refresh(source.id);
    } else if (action === 'toggle') {
      await toggle(source.id, !source.enabled);
    } else if (action === 'details') {
      openDetails(source.id);
    } else if (action === 'remove') {
      if (!window.confirm(`Удалить источник «${source.name}»?`)) return;
      await remove(source.id);
    }
  }
</script>

<PageHeader
  title="Источники"
  subtitle="Источники — это фиды с релизами игр, которые Typhon использует для поиска и сопоставления загрузок. Источники обновляются автоматически и вручную."
>
  {#snippet actions()}
    <Button disabled={$refreshingAll} onclick={refreshAll}>
      <span class="spin" class:on={$refreshingAll}>
        <RefreshCw size="1.6rem" strokeWidth={1.8} />
      </span>
      {$refreshingAll ? 'Обновление…' : 'Обновить все'}
    </Button>
    <Button variant="primary" onclick={() => (addOpen = true)}>
      <Plus size="1.6rem" strokeWidth={2} />
      Добавить источник
    </Button>
  {/snippet}
</PageHeader>

{#if $sources.length === 0}
  <div class="empty-wrap">
    <EmptyState
      title="Источники ещё не добавлены"
      description="Добавьте первый источник, чтобы Typhon мог находить релизы игр и обновления."
    >
      {#snippet icon()}
        <Database size="2.2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => (addOpen = true)}>
          <Plus size="1.6rem" strokeWidth={2} />
          Добавить источник
        </Button>
      {/snippet}
    </EmptyState>
  </div>
{:else}
  <div class="table">
    <div class="thead">
      <span class="th">Источник</span>
      <span class="th">Статус</span>
      <span class="th center">Здоровье</span>
      <span class="th">Обновлён</span>
      <span class="th nums">Записей</span>
      <span class="th">Сопоставление</span>
      <span class="th"></span>
    </div>
    {#each $sources as source (source.id)}
      {@const badge = statusBadge[source.status]}
      {@const health = healthMeta[source.health]}
      <div class="row" class:disabled={!source.enabled}>
        <button class="source" onclick={() => openDetails(source.id)}>
          <div class="source-icon">
            <Globe size="1.8rem" strokeWidth={1.8} />
          </div>
          <div class="source-text">
            <span class="source-name">{source.name}</span>
            <span class="source-url">{source.url}</span>
          </div>
        </button>
        <span class="cell">
          <StatusBadge kind={badge.kind} label={badge.label} />
          {#if source.lastError}
            <Tooltip text={truncateMiddle(source.lastError, 90)}>
              <span class="warn-icon"><CircleAlert size="1.5rem" strokeWidth={1.8} /></span>
            </Tooltip>
          {/if}
        </span>
        <span class="cell center">
          <Tooltip text={source.warnings?.length ? `${health.label}: ${source.warnings.join('; ')}` : health.label}>
            <span class="health" style:color={health.color}>
              <health.icon size="1.8rem" strokeWidth={1.8} />
            </span>
          </Tooltip>
        </span>
        <span class="cell">{relativeDate(source.lastUpdatedAt)}</span>
        <span class="cell nums">{formatCount(source.entries)}</span>
        <span class="cell counts">
          <span class="count success" title="Сопоставлено">{formatCount(source.matched)}</span>
          <span class="count warning" title="На проверку">{formatCount(source.review)}</span>
          <span class="count neutral" title="Без совпадения">{formatCount(source.unmatched)}</span>
        </span>
        <span class="cell menu-cell">
          <DropdownMenu items={sourceMenu(source)} onselect={(id) => onSourceMenu(source, id)}>
            {#snippet trigger({ toggle: toggleMenu })}
              <IconButton label="Меню источника" size="sm" onclick={toggleMenu}>
                <EllipsisVertical size="1.6rem" strokeWidth={1.8} />
              </IconButton>
            {/snippet}
          </DropdownMenu>
        </span>
      </div>
    {/each}
  </div>
{/if}

<AddSourceModal bind:open={addOpen} />
<SourceDetailsModal bind:open={detailsOpen} sourceId={detailsId} />

<style>
  .empty-wrap {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .table {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }

  .thead,
  .row {
    display: grid;
    grid-template-columns: minmax(24rem, 1.6fr) 12rem 6.4rem 11rem 8rem 15rem 5.2rem;
    align-items: center;
    gap: var(--space-4);
    padding: 0 var(--space-5);
  }

  .thead {
    height: 4.6rem;
    border-bottom: 1px solid var(--border);
  }

  .th {
    font-size: 1.3rem;
    font-weight: 500;
    color: var(--text-3);
  }

  .th.center {
    text-align: center;
  }

  .th.nums {
    text-align: right;
  }

  .row {
    padding-top: 1.3rem;
    padding-bottom: 1.3rem;
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: rgba(255, 255, 255, 0.02);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row.disabled .source-name,
  .row.disabled .cell {
    color: var(--text-3);
  }

  .source {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
    text-align: left;
  }

  .source-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4rem;
    height: 4rem;
    border-radius: var(--radius-md);
    flex-shrink: 0;
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .source-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .source-name {
    font-size: 1.5rem;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: color var(--dur) var(--ease);
  }

  .source:hover .source-name {
    color: var(--accent-text);
  }

  .source-url {
    font-size: 1.3rem;
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-size: 1.4rem;
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell.nums {
    justify-content: flex-end;
    font-variant-numeric: tabular-nums;
  }

  .cell.center {
    justify-content: center;
    overflow: visible;
  }

  .warn-icon {
    display: inline-flex;
    color: var(--danger);
  }

  .health {
    display: inline-flex;
  }

  .counts {
    overflow: visible;
  }

  .count {
    font-size: 1.4rem;
    font-variant-numeric: tabular-nums;
  }

  .count.success {
    color: var(--success);
  }

  .count.warning {
    color: var(--warning);
  }

  .count.neutral {
    color: var(--text-3);
  }

  .count + .count::before {
    content: '/';
    margin-right: 0.6rem;
    color: var(--text-3);
    opacity: 0.5;
  }

  .menu-cell {
    overflow: visible;
    justify-self: end;
  }

  .spin {
    display: inline-flex;
  }

  .spin.on :global(svg) {
    animation: spin 900ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (max-width: 1300px) {
    .thead,
    .row {
      grid-template-columns: minmax(20rem, 1.6fr) 11rem 6.4rem 9rem 15rem 5.2rem;
    }

    .th:nth-child(5),
    .cell.nums {
      display: none;
    }
  }
</style>
