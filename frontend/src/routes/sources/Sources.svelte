<script lang="ts">
  import { untrack } from 'svelte';
  import {
    CircleAlert,
    CircleCheck,
    Database,
    EllipsisVertical,
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
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { route } from '../../lib/stores/router';
  import { refresh, refreshAll, refreshingAll, remove, sources, toggle } from '../../lib/stores/sources';
  import { needsSourcesNotice } from '../../lib/stores/sourcesNotice';
  import { sourceLocation, type Source, type SourceHealth, type SourceStatus } from '../../lib/services/sources';
  import { formatCount, relativeDate, truncateMiddle } from '../../lib/utils/format';

  let addOpen = $state(false);
  let noticeOpen = $state(false);
  let detailsOpen = $state(false);
  let detailsId = $state<string | null>(null);
  let detailsReleaseId = $state<string | null>(null);

  function startAddSource() {
    if (needsSourcesNotice()) {
      noticeOpen = true;
    } else {
      addOpen = true;
    }
  }

  $effect(() => {
    const params = $route.params;
    untrack(() => {
      if (!params.sourceId) return;
      detailsId = params.sourceId;
      detailsReleaseId = params.releaseId ?? null;
      detailsOpen = true;
    });
  });

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
    detailsReleaseId = null;
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

<PageHeader title="Источники" subtitle="Пользовательские источники релизов">
  {#snippet actions()}
    <Button variant="ghost" disabled={$refreshingAll} onclick={refreshAll}>
      <span class="spin" class:on={$refreshingAll}>
        <RefreshCw size="1.5rem" strokeWidth={1.8} />
      </span>
      {$refreshingAll ? 'Обновление…' : 'Обновить все'}
    </Button>
    <Button variant="primary" onclick={startAddSource}>
      <Plus size="1.5rem" strokeWidth={2} />
      Добавить источник
    </Button>
  {/snippet}
</PageHeader>

<p class="device-hint">Добавленные источники обрабатываются на вашем устройстве.</p>

{#if $sources.length === 0}
  <EmptyState
    title="Источники ещё не добавлены"
    description="Добавьте первый источник, чтобы Typhon мог находить релизы игр и обновления."
  >
    {#snippet icon()}
      <Database size="2rem" strokeWidth={1.8} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={startAddSource}>
        <Plus size="1.5rem" strokeWidth={2} />
        Добавить источник
      </Button>
    {/snippet}
  </EmptyState>
{:else}
  <div class="table">
    <div class="thead">
      <span class="th">Источник</span>
      <span class="th">Статус</span>
      <span class="th">Обновлён</span>
      <span class="th nums">Записей</span>
      <span class="th nums">Сопоставлено</span>
      <span class="th"></span>
    </div>
    {#each $sources as source (source.id)}
      {@const badge = statusBadge[source.status]}
      {@const health = healthMeta[source.health]}
      <div class="row" class:disabled={!source.enabled}>
        <button class="source" onclick={() => openDetails(source.id)}>
          <span class="source-name">{source.name}</span>
          <span class="source-url" title={sourceLocation(source)}>{sourceLocation(source)}</span>
        </button>
        <span class="cell status">
          <StatusBadge kind={badge.kind} label={badge.label} plain />
          {#if source.lastError}
            <Tooltip text={truncateMiddle(source.lastError, 90)}>
              <span class="warn-icon"><CircleAlert size="1.5rem" strokeWidth={1.8} /></span>
            </Tooltip>
          {:else if source.health !== 'healthy'}
            <Tooltip text={source.warnings?.length ? `${health.label}: ${source.warnings.join('; ')}` : health.label}>
              <span class="health" style:color={health.color}>
                <health.icon size="1.5rem" strokeWidth={1.8} />
              </span>
            </Tooltip>
          {/if}
        </span>
        <span class="cell">{relativeDate(source.lastUpdatedAt)}</span>
        <span class="cell nums">{formatCount(source.entries)}</span>
        <span class="cell nums counts">
          <Tooltip text={`Сопоставлено ${formatCount(source.matched)} · на проверку ${formatCount(source.review)} · без совпадения ${formatCount(source.unmatched)}`}>
            <span class="count-wrap">
              <span class="count">{formatCount(source.matched)}</span>
              {#if source.review > 0}
                <span class="count review">+{formatCount(source.review)}</span>
              {/if}
            </span>
          </Tooltip>
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
<SourcesNoticeModal bind:open={noticeOpen} onaccepted={() => (addOpen = true)} />
<SourceDetailsModal bind:open={detailsOpen} sourceId={detailsId} focusReleaseId={detailsReleaseId} />

<style>
  .device-hint {
    margin: calc(var(--space-6) * -1) 0 var(--space-6);
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .table {
    display: flex;
    flex-direction: column;
    max-width: 140rem;
  }

  .thead,
  .row {
    display: grid;
    grid-template-columns: minmax(24rem, 1.6fr) 16rem 11rem 9rem 11rem 4rem;
    align-items: center;
    gap: var(--space-5);
  }

  .thead {
    padding: 0 1.2rem 0.8rem;
    border-bottom: 1px solid var(--border);
  }

  .th {
    font-size: 1.2rem;
    font-weight: 500;
    color: var(--text-3);
    white-space: nowrap;
  }

  .th.nums {
    text-align: right;
  }

  .row {
    padding: 1.2rem 1.2rem;
    transition: background var(--dur) var(--ease);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row:hover {
    background: var(--hover);
  }

  .row.disabled .source-name,
  .row.disabled .cell {
    color: var(--text-3);
  }

  .source {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
    text-align: left;
  }

  .source-name {
    font-size: var(--font-md);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: color var(--dur) var(--ease);
  }

  .source:hover .source-name {
    color: var(--accent-text);
  }

  .source-url {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell.status,
  .cell.counts,
  .cell.menu-cell {
    overflow: visible;
  }

  .cell.nums {
    justify-content: flex-end;
    font-variant-numeric: tabular-nums;
  }

  .warn-icon {
    display: inline-flex;
    color: var(--danger);
  }

  .health {
    display: inline-flex;
  }

  .count-wrap {
    display: inline-flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  .count {
    font-variant-numeric: tabular-nums;
  }

  .count.review {
    font-size: var(--font-xs);
    color: var(--warning);
  }

  .menu-cell {
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
      grid-template-columns: minmax(20rem, 1.6fr) 15rem 10rem 11rem 4rem;
    }

    .th:nth-child(4),
    .cell.nums:not(.counts) {
      display: none;
    }
  }
</style>
