<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { sourceWarningText } from '../../lib/sources/sourceWarnings';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';
  import { untrack } from 'svelte';
  import {
    CircleAlert,
    CircleCheck,
    Database,
    EllipsisVertical,
    FileText,
    Globe,
    Plus,
    RefreshCw,
    ShieldAlert,
    TriangleAlert,
  } from '@lucide/svelte';
  import AddSourceModal from '../../lib/components/AddSourceModal.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import SourceDetailsModal from '../../lib/components/SourceDetailsModal.svelte';
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { route } from '../../lib/stores/router';
  import { removeSourcePrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import { refresh, refreshAll, refreshingAll, remove, sources, toggle } from '../../lib/stores/sources';
  import { needsSourcesNotice } from '../../lib/stores/sourcesNotice';
  import { sourceProgress } from '../../lib/stores/sourceProgress';
  import { isIndeterminate, overallPercent, progressPercent, type SourceProgress } from '../../lib/sources/progress';
  import { sourceLocation, type Source, type SourceHealth, type SourceStatus } from '../../lib/services/sources';
  import { clockTime, formatCount, numericDate, relativeDate, truncateMiddle } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  let addOpen = $state(false);
  let noticeOpen = $state(false);
  let detailsOpen = $state(false);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let detailsId = $state<string | null>(null);
  let detailsReleaseId = $state<string | null>(null);
  let statusFilter = $state('all');

  const progressList = $derived(Object.values($sourceProgress));
  const allPercent = $derived(Math.round(overallPercent(progressList)));

  function stageLabel(p: SourceProgress): string {
    const counts = { done: formatCount(p.done), total: formatCount(p.total) };
    switch (p.stage) {
      case 'fetch':
        return msg('transfers.sourcesStageFetch');
      case 'parse':
        return msg('transfers.sourcesStageParse', counts);
      case 'match':
        return msg('transfers.sourcesStageMatch', counts);
      case 'apply':
        return msg('transfers.sourcesStageApply');
      case 'save':
        return msg('transfers.sourcesStageSave');
    }
  }

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

  function healthMetaFor(health: SourceHealth): { icon: typeof CircleCheck; kind: 'success' | 'warning' | 'danger'; label: string } {
    switch (health) {
      case 'healthy':
        return { icon: CircleCheck, kind: 'success', label: msg('transfers.sourcesHealthOk') };
      case 'warning':
        return { icon: TriangleAlert, kind: 'warning', label: msg('transfers.sourcesHealthWarning') };
      case 'error':
        return { icon: CircleAlert, kind: 'danger', label: msg('transfers.sourcesHealthError') };
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

  function exactTime(iso: string | null) {
    if (!iso) return undefined;
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? undefined : `${numericDate(date)}, ${clockTime(date)}`;
  }

  function openDetails(id: string) {
    detailsId = id;
    detailsReleaseId = null;
    detailsOpen = true;
  }

  function sourceMenu(source: Source) {
    return [
      {
        id: 'refresh',
        label: source.status === 'updating' ? msg('transfers.sourcesUpdatingEllipsis') : msg('transfers.sourcesRefreshNow'),
        disabled: source.status === 'updating',
      },
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
      pending = { prompt: removeSourcePrompt(source.name), run: () => remove(source.id) };
    }
  }
</script>

<Page>
  <PageHeader title={msg('transfers.sourcesTitle')} subtitle={msg('transfers.sourcesSubtitle')}>
    {#snippet actions()}
      <Button variant="ghost" busy={$refreshingAll} onclick={refreshAll}>
        {#if !$refreshingAll}<RefreshCw size="1.5rem" strokeWidth={1.8} />{/if}
        {#if $refreshingAll && progressList.length > 0}
          <span class="all-progress">
            <span>{msg('transfers.sourcesRefreshAllProgress', { percent: allPercent })}</span>
            <ProgressBar value={allPercent} height={3} decorative />
          </span>
        {:else}
          {$refreshingAll ? msg('transfers.sourcesUpdatingEllipsis') : msg('transfers.sourcesRefreshAll')}
        {/if}
      </Button>
      <Button variant="primary" onclick={startAddSource}>
        <Plus size="1.5rem" strokeWidth={2} />
        {msg('transfers.sourcesAddAction')}
      </Button>
    {/snippet}
  </PageHeader>

  <div class="fetched-here">
    <Notice tone="info" announce={false}>{msg('transfers.sourcesNotice')}</Notice>
  </div>

  {#if $sources.length === 0}
    <EmptyState level={2}
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
    <SegmentedControl options={statusTabs} label={msg('transfers.sourcesStatusFilterLabel')} variant="pill" bind:value={statusFilter} />

    {#if filteredSources.length === 0}
      <EmptyState level={2} title={msg('transfers.sourcesEmptyFilteredTitle')} description={msg('transfers.sourcesEmptyFilteredDescription')} />
    {:else}
      {#key statusFilter}
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
          {@const live = source.status === 'updating' ? $sourceProgress[source.id] : undefined}
          <div class="row" class:disabled={!source.enabled}>
            {#snippet details()}
              <button class="source" onclick={() => openDetails(source.id)}>
                <span class="type-icon">
                  {#if source.type === 'file'}
                    <FileText size="1.7rem" strokeWidth={1.8} />
                  {:else}
                    <Globe size="1.7rem" strokeWidth={1.8} />
                  {/if}
                </span>
                <span class="source-text">
                  <span class="source-name">
                    {source.name}
                    {#if source.insecure}
                      <span class="insecure"><ShieldAlert size="1.4rem" strokeWidth={1.8} /></span>
                    {/if}
                  </span>
                  <span class="source-url" title={sourceLocation(source)}>{sourceLocation(source)}</span>
                  {#if source.lastError}
                    <span class="source-error" title={sourceErrorText(source.lastError)}>{sourceErrorText(source.lastError)}</span>
                  {/if}
                </span>
              </button>
            {/snippet}
            {#if source.insecure}
              <Tooltip text={msg('transfers.sourcesInsecureTooltip')}>{@render details()}</Tooltip>
            {:else}
              {@render details()}
            {/if}
            <span class="cell status">
              {#if live}
                {@const percent = Math.round(progressPercent(live))}
                <span class="live" title={stageLabel(live)}>
                  <span class="live-label">{stageLabel(live)}</span>
                  <span class="live-bar">
                    <ProgressBar value={percent} indeterminate={isIndeterminate(live)} height={4} label={source.name} valueText={`${stageLabel(live)}, ${percent}%`} />
                    {#if !isIndeterminate(live)}<span class="live-percent">{percent}%</span>{/if}
                  </span>
                </span>
              {:else}
                <StatusBadge kind={badge.kind} label={badge.label} plain />
                {#if source.status === 'updating'}
                  <Spinner size="sm" />
                {/if}
              {/if}
              {#if source.lastError}
                <IconButton size="sm" label={msg('common.error')} hint={truncateMiddle(sourceErrorText(source.lastError), 90)} onclick={() => openDetails(source.id)}>
                  <span class="mark danger"><CircleAlert size="1.5rem" strokeWidth={1.8} /></span>
                </IconButton>
              {:else if source.health !== 'healthy'}
                {@const warnings = source.warnings?.length ? source.warnings.map(sourceWarningText).join('; ') : ''}
                <IconButton size="sm" label={health.label} hint={warnings ? `${health.label}: ${warnings}` : undefined} onclick={() => openDetails(source.id)}>
                  <span class="mark {health.kind}"><health.icon size="1.5rem" strokeWidth={1.8} /></span>
                </IconButton>
              {/if}
            </span>
            <span class="cell" title={exactTime(source.lastUpdatedAt)}>{relativeDate(source.lastUpdatedAt)}</span>
            <span class="cell nums">{formatCount(source.entries)}</span>
            <span class="cell nums counts">
              <Tooltip
                text={msg('transfers.sourcesMatchTooltip', {
                  matched: formatCount(source.matched),
                  review: formatCount(source.review),
                  unmatched: formatCount(source.unmatched),
                })}
              >
                <TextLink
                  aria-label={source.review > 0
                    ? msg('transfers.sourcesMatchReviewName', { matched: formatCount(source.matched), review: formatCount(source.review) })
                    : msg('transfers.sourcesMatchName', { matched: formatCount(source.matched) })}
                  onclick={() => openDetails(source.id)}
                >
                  <span class="count-wrap">
                    <span class="count">{formatCount(source.matched)}</span>
                    {#if source.review > 0}
                      <span class="count review">+{formatCount(source.review)}</span>
                    {/if}
                  </span>
                </TextLink>
              </Tooltip>
            </span>
            <span class="cell menu-cell">
              <DropdownMenu items={sourceMenu(source)} onselect={(id) => onSourceMenu(source, id)}>
                {#snippet trigger({ props })}
                  <IconButton {...props} label={msg('transfers.sourcesMenuFor', { name: source.name })} size="sm">
                    <EllipsisVertical size="1.6rem" strokeWidth={1.8} />
                  </IconButton>
                {/snippet}
              </DropdownMenu>
            </span>
          </div>
        {/each}
      </div>
      {/key}
    {/if}
  {/if}

  <AddSourceModal bind:open={addOpen} />

  <SourcesNoticeModal bind:open={noticeOpen} onaccepted={() => (addOpen = true)} />

  <SourceDetailsModal bind:open={detailsOpen} sourceId={detailsId} focusReleaseId={detailsReleaseId} />

  {#if pending}
    <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} errorText={(err) => sourceErrorText(err)} />
  {/if}
</Page>

<style>
  .fetched-here {
    margin-bottom: var(--space-5);
  }

  .table {
    display: flex;
    flex-direction: column;
    max-width: 140rem;
    margin-top: var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .row:last-child {
    border-radius: 0 0 var(--radius-lg) var(--radius-lg);
  }

  .source-error {
    font-size: var(--font-xs);
    color: var(--danger);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .all-progress {
    display: inline-flex;
    flex-direction: column;
    gap: 0.4rem;
    min-width: 12rem;
    text-align: left;
  }

  .live {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-width: 0;
    width: 100%;
  }

  .live-label {
    font-size: var(--font-xs);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .live-bar {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .live-percent {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .thead,
  .row {
    display: grid;
    grid-template-columns: minmax(24rem, 1.6fr) 16rem 11rem 9.5rem 12rem 4rem;
    align-items: center;
    gap: var(--space-5);
  }

  .thead {
    padding: 1rem 1.2rem;
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

  .insecure {
    display: inline-flex;
    align-items: center;
    color: var(--warning);
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
  .cell.counts {
    overflow: visible;
  }

  /* The menu button's focus ring is drawn outside it. */
  .cell.menu-cell {
    overflow: visible;
  }

  .cell.nums {
    justify-content: flex-end;
    font-variant-numeric: tabular-nums;
  }

  .mark {
    display: inline-flex;
  }

  .mark.danger {
    color: var(--danger);
  }

  .mark.warning {
    color: var(--warning);
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
