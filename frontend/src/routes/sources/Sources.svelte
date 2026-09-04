<script lang="ts">
  import { untrack } from 'svelte';
  import {
    CircleAlert,
    CircleCheck,
    Database,
    EllipsisVertical,
    FileText,
    Globe,
    Info,
    Plus,
    RefreshCw,
    TriangleAlert,
  } from '@lucide/svelte';
  import AddSourceModal from '../../lib/components/AddSourceModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SourceDetailsModal from '../../lib/components/SourceDetailsModal.svelte';
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { route } from '../../lib/stores/router';
  import { refresh, refreshAll, refreshingAll, remove, sources, toggle } from '../../lib/stores/sources';
  import { needsSourcesNotice } from '../../lib/stores/sourcesNotice';
  import { sourceLocation, type Source, type SourceHealth, type SourceStatus } from '../../lib/services/sources';
  import { formatCount, relativeDate, truncateMiddle } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  let addOpen = $state(false);
  let noticeOpen = $state(false);
  let detailsOpen = $state(false);
  let detailsId = $state<string | null>(null);
  let detailsReleaseId = $state<string | null>(null);
  let statusFilter = $state('all');

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

  function statusBadgeFor(status: SourceStatus): { kind: 'success' | 'neutral' | 'danger'; label: string } {
    switch (status) {
      case 'active':
        return { kind: 'success', label: msg('transfers.sourcesStatusActive') };
      case 'disabled':
        return { kind: 'neutral', label: msg('transfers.sourcesStatusDisabled') };
      case 'error':
        return { kind: 'danger', label: msg('common.error') };
      case 'updating':
        return { kind: 'neutral', label: msg('transfers.sourcesStatusUpdating') };
    }
  }

  function statusTabLabel(status: SourceStatus): string {
    switch (status) {
      case 'active':
        return msg('transfers.sourcesTabActive');
      case 'disabled':
        return msg('transfers.sourcesTabDisabled');
      case 'error':
        return msg('transfers.sourcesTabError');
      case 'updating':
        return msg('transfers.sourcesTabUpdating');
    }
  }

  const statusOrder: SourceStatus[] = ['active', 'disabled', 'error', 'updating'];

  function healthMetaFor(health: SourceHealth): { icon: typeof CircleCheck; color: string; label: string } {
    switch (health) {
      case 'healthy':
        return { icon: CircleCheck, color: 'var(--success)', label: msg('transfers.sourcesHealthOk') };
      case 'warning':
        return { icon: TriangleAlert, color: 'var(--warning)', label: msg('transfers.sourcesHealthWarning') };
      case 'error':
        return { icon: CircleAlert, color: 'var(--danger)', label: msg('transfers.sourcesHealthError') };
    }
  }

  const statusTabs = $derived([
    { id: 'all', label: msg('transfers.sourcesTabAll'), count: $sources.length },
    ...statusOrder.map((status) => ({
      id: status,
      label: statusTabLabel(status),
      count: $sources.filter((source) => source.status === status).length,
    })),
  ]);

  const filteredSources = $derived(
    statusFilter === 'all' ? $sources : $sources.filter((source) => source.status === statusFilter),
  );

  function openDetails(id: string) {
    detailsId = id;
    detailsReleaseId = null;
    detailsOpen = true;
  }

  function sourceMenu(source: Source) {
    return [
      { id: 'refresh', label: source.status === 'updating' ? msg('transfers.sourcesUpdatingEllipsis') : msg('transfers.sourcesRefreshNow') },
      { id: 'toggle', label: source.enabled ? msg('transfers.sourcesDisableAction') : msg('transfers.sourcesEnableAction') },
      { id: 'details', label: msg('transfers.sourcesDetailsAction') },
      { id: 'remove', label: msg('transfers.sourcesRemoveAction'), danger: true, separator: true },
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
      if (!window.confirm(msg('transfers.sourcesConfirmRemove', { name: source.name }))) return;
      await remove(source.id);
    }
  }
</script>

<Card surface="panel">
  <PageHeader title={msg('transfers.sourcesTitle')} subtitle={msg('transfers.sourcesSubtitle')}>
    {#snippet actions()}
      <Button variant="ghost" disabled={$refreshingAll} onclick={refreshAll}>
        <span class="spin" class:on={$refreshingAll}>
          <RefreshCw size="1.5rem" strokeWidth={1.8} />
        </span>
        {$refreshingAll ? msg('transfers.sourcesUpdatingEllipsis') : msg('transfers.sourcesRefreshAll')}
      </Button>
      <Button variant="primary" onclick={startAddSource}>
        <Plus size="1.5rem" strokeWidth={2} />
        {msg('transfers.sourcesAddAction')}
      </Button>
    {/snippet}
  </PageHeader>

  <div class="notice">
    <span class="notice-icon"><Info size="1.8rem" strokeWidth={1.8} /></span>
    <p class="notice-text">{msg('transfers.sourcesNotice')}</p>
  </div>

  {#if $sources.length === 0}
    <EmptyState
      title={msg('transfers.sourcesEmptyTitle')}
      description={msg('transfers.sourcesEmptyDescription')}
    >
      {#snippet icon()}
        <Database size="2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={startAddSource}>
          <Plus size="1.5rem" strokeWidth={2} />
          {msg('transfers.sourcesAddAction')}
        </Button>
      {/snippet}
    </EmptyState>
  {:else}
    <Tabs tabs={statusTabs} bind:value={statusFilter} variant="pill" />

    {#if filteredSources.length === 0}
      <EmptyState title={msg('transfers.sourcesEmptyFilteredTitle')} description={msg('transfers.sourcesEmptyFilteredDescription')} />
    {:else}
      <div class="table">
        <div class="thead">
          <span class="th">{msg('transfers.sourcesColName')}</span>
          <span class="th">{msg('transfers.sourcesColStatus')}</span>
          <span class="th">{msg('transfers.sourcesColUpdated')}</span>
          <span class="th nums">{msg('transfers.sourcesColEntries')}</span>
          <span class="th nums">{msg('transfers.sourcesColMatched')}</span>
          <span class="th"></span>
        </div>
        {#each filteredSources as source (source.id)}
          {@const badge = statusBadgeFor(source.status)}
          {@const health = healthMetaFor(source.health)}
          <div class="row" class:disabled={!source.enabled}>
            <button class="source" onclick={() => openDetails(source.id)}>
              <span class="type-icon">
                {#if source.type === 'file'}
                  <FileText size="1.7rem" strokeWidth={1.8} />
                {:else}
                  <Globe size="1.7rem" strokeWidth={1.8} />
                {/if}
              </span>
              <span class="source-text">
                <span class="source-name">{source.name}</span>
                <span class="source-url" title={sourceLocation(source)}>{sourceLocation(source)}</span>
              </span>
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
              <Tooltip
                text={msg('transfers.sourcesMatchTooltip', {
                  matched: formatCount(source.matched),
                  review: formatCount(source.review),
                  unmatched: formatCount(source.unmatched),
                })}
              >
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
                  <IconButton label={msg('transfers.sourcesMenuLabel')} size="sm" onclick={toggleMenu}>
                    <EllipsisVertical size="1.6rem" strokeWidth={1.8} />
                  </IconButton>
                {/snippet}
              </DropdownMenu>
            </span>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</Card>

<AddSourceModal bind:open={addOpen} />
<SourcesNoticeModal bind:open={noticeOpen} onaccepted={() => (addOpen = true)} />
<SourceDetailsModal bind:open={detailsOpen} sourceId={detailsId} focusReleaseId={detailsReleaseId} />

<style>
  .notice {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    margin-bottom: var(--space-6);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text-3);
  }

  .notice-icon {
    display: flex;
    flex-shrink: 0;
    margin-top: 0.1rem;
    color: var(--text-3);
  }

  .notice-text {
    font-size: var(--font-xs);
    line-height: 1.5;
  }

  .table {
    display: flex;
    flex-direction: column;
    max-width: 140rem;
    margin-top: var(--space-5);
  }

  .thead,
  .row {
    display: grid;
    grid-template-columns: minmax(24rem, 1.6fr) 16rem 11rem 9.5rem 12rem 4rem;
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
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
    text-align: left;
  }

  .type-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.6rem;
    height: 3.6rem;
    flex-shrink: 0;
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .source-text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
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
    white-space: nowrap;
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
      grid-template-columns: minmax(20rem, 1.6fr) 15rem 10rem 12rem 4rem;
    }

    .th:nth-child(4),
    .cell.nums:not(.counts) {
      display: none;
    }
  }
</style>
