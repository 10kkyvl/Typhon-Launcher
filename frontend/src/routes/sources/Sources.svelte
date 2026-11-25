<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';
  import { untrack } from 'svelte';
  import { Database, Plus, RefreshCw } from '@lucide/svelte';
  import AddSourceModal from '../../lib/components/AddSourceModal.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import SourceDetailsModal from '../../lib/components/SourceDetailsModal.svelte';
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { route } from '../../lib/stores/router';
  import { removeSourcePrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import {
    refresh,
    refreshAll,
    refreshingAll,
    reloadSources,
    remove,
    sources,
    sourcesLoaded,
    sourcesLoadError,
    toggle,
  } from '../../lib/stores/sources';
  import { needsSourcesNotice } from '../../lib/stores/sourcesNotice';
  import { sourceProgress } from '../../lib/stores/sourceProgress';
  import { overallPercent } from '../../lib/sources/progress';
  import type { Source, SourceStatus } from '../../lib/services/sources';
  import { msg } from '../../lib/i18n';
  import SourceRow from './SourceRow.svelte';
  import { catchLeaving, placeOf, returnTo, type RowPlace } from './rowFocus';

  let addOpen = $state(false);
  let noticeOpen = $state(false);
  let detailsOpen = $state(false);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void>; place: RowPlace | null } | null>(null);
  let detailsId = $state<string | null>(null);
  let detailsReleaseId = $state<string | null>(null);
  let statusFilter = $state('all');
  let retrying = $state(false);
  // The state each switch asked Go for, by source, until Go answers; then the source's own shows again.
  let asked = $state<Record<string, boolean>>({});
  let list = $state<HTMLElement>();

  const loadFailed = $derived(!$sourcesLoaded && $sourcesLoadError != null);
  const loading = $derived(!$sourcesLoaded && !loadFailed);

  const progressList = $derived(Object.values($sourceProgress));
  const allPercent = $derived(Math.round(overallPercent(progressList)));

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

  // Before any change of the rows shown (a removal, a source Go drops, a row the filter no longer
  // shows), the screen's rule reads where focus is, and hands it on if the focused row went.
  $effect.pre(() => {
    void filteredSources;
    void $sourcesLoaded;
    untrack(catchLeaving);
  });

  function openDetails(id: string) {
    detailsId = id;
    detailsReleaseId = null;
    detailsOpen = true;
  }

  /** A refusal is said once, by the store; the switch then shows the source as Go still holds it. */
  async function onToggle(source: Source, enabled: boolean) {
    if (source.id in asked) return;
    asked = { ...asked, [source.id]: enabled };
    try {
      await toggle(source.id, enabled);
    } finally {
      const next = { ...asked };
      delete next[source.id];
      asked = next;
    }
  }

  async function onSourceMenu(source: Source, action: string) {
    if (action === 'refresh') {
      if (source.status === 'updating') return;
      await refresh(source.id);
    } else if (action === 'details') {
      openDetails(source.id);
    } else if (action === 'remove') {
      pending = { prompt: removeSourcePrompt(source.name), run: () => remove(source.id), place: placeOf(document.activeElement) };
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
      <Button variant="primary" data-sources-add onclick={startAddSource}>
        <Plus size="1.5rem" strokeWidth={2} />
        {msg('transfers.sourcesAddAction')}
      </Button>
    {/snippet}
  </PageHeader>

  <div class="fetched-here">
    <Notice tone="info" announce={false}>{msg('transfers.sourcesNotice')}</Notice>
  </div>

  {#if loadFailed || retrying}
    <!-- On success focus that was on Retry goes to the first control of the list, else the page heading. -->
    <div class="load-failed">
      <Notice tone="danger" bind:retrying retry={{ label: msg('common.retry'), run: reloadSources, within: () => list }}>
        {sourceErrorText($sourcesLoadError, msg('transfers.sourcesLoadFailed'))}
      </Notice>
    </div>
  {/if}

  {#if !$sourcesLoaded}
    {#if loading}
      <EmptyState level={2} title={msg('transfers.sourcesLoading')} />
    {/if}
  {:else if $sources.length === 0}
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
      <ul class="list" aria-label={msg('transfers.sourcesTitle')} data-source-list bind:this={list}>
        {#each filteredSources as source (source.id)}
          <SourceRow
            {source}
            live={source.status === 'updating' ? $sourceProgress[source.id] : undefined}
            enabled={asked[source.id] ?? source.enabled}
            onopen={openDetails}
            ontoggle={(enabled) => onToggle(source, enabled)}
            onaction={(action) => onSourceMenu(source, action)}
          />
        {/each}
      </ul>
      {/key}
    {/if}
  {/if}

  <AddSourceModal bind:open={addOpen} />

  <SourcesNoticeModal bind:open={noticeOpen} onaccepted={() => (addOpen = true)} />

  <SourceDetailsModal bind:open={detailsOpen} sourceId={detailsId} focusReleaseId={detailsReleaseId} />

  {#if pending}
    {@const place = pending.place}
    <ConfirmModal
      prompt={pending.prompt}
      onconfirm={pending.run}
      onclose={() => (pending = null)}
      errorText={(err) => sourceErrorText(err)}
      returnFocus={() => returnTo(place)}
    />
  {/if}
</Page>

<style>
  .fetched-here {
    margin-bottom: var(--space-5);
  }

  .load-failed {
    margin-bottom: var(--space-5);
  }

  .list {
    display: flex;
    flex-direction: column;
    max-width: 140rem;
    margin: var(--space-5) 0 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
    overflow: hidden;
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .all-progress {
    display: inline-flex;
    flex-direction: column;
    gap: 0.4rem;
    min-width: 12rem;
    text-align: left;
  }
</style>
