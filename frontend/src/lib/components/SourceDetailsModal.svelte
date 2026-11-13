<script lang="ts">
  import { sourceErrorText } from '../sources/sourceErrors';
  import { tick, untrack } from 'svelte';
  import { ChevronLeft, ChevronRight } from '@lucide/svelte';
  import {
    getRelease,
    getSourceDetails,
    queryReleases,
    sourceLocation,
    type ReleaseView,
    type SourceDetails,
  } from '../services/sources';
  import { LatestRequestGate } from '../bigpicture/latestRequest';
  import { removeSourcePrompt, type ConfirmPrompt } from '../confirm/prompts';
  import { toast } from '../stores/toasts';
  import { refresh as refreshSource, remove as removeSource, sources, toggle as toggleSource } from '../stores/sources';
  import { relativeDate, bytesSize } from '../utils/format';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import ReleaseMatchModal from './ReleaseMatchModal.svelte';
  import SearchInput from './SearchInput.svelte';
  import Select from './Select.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    sourceId,
    focusReleaseId = null,
  }: { open?: boolean; sourceId: string | null; focusReleaseId?: string | null } = $props();

  const source = $derived(sourceId ? $sources.find((s) => s.id === sourceId) : undefined);

  const statusOptions = $derived([
    { id: 'all', label: msg('modals.sourceDetailsStatusAll') },
    { id: 'matched', label: msg('modals.sourceDetailsStatusMatched') },
    { id: 'review', label: msg('modals.sourceDetailsStatusReview') },
    { id: 'unmatched', label: msg('modals.sourceDetailsStatusUnmatched') },
    { id: 'removed', label: msg('modals.sourceDetailsStatusRemoved') },
    { id: 'new', label: msg('modals.sourceDetailsStatusNew') },
  ]);

  function statusBadgeInfo(status: string): { kind: 'success' | 'warning' | 'neutral'; label: string } | undefined {
    switch (status) {
      case 'active':
        return { kind: 'success', label: msg('modals.sourceDetailsStatusActive') };
      case 'disabled':
        return { kind: 'neutral', label: msg('modals.sourceDetailsStatusDisabled') };
      case 'error':
        return { kind: 'neutral', label: msg('common.error') };
      case 'updating':
        return { kind: 'neutral', label: msg('modals.sourceDetailsStatusUpdating') };
      default:
        return undefined;
    }
  }

  const detailsRequests = new LatestRequestGate();
  const releaseRequests = new LatestRequestGate();
  let details = $state<SourceDetails | null>(null);
  let detailsError = $state('');
  let releasesError = $state('');
  let filterStatus = $state('all');
  let search = $state('');
  let page = $state(1);
  const pageSize = 50;
  let releases = $state<ReleaseView[]>([]);
  let total = $state(0);
  let loadingReleases = $state(false);
  let refreshing = $state(false);
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  let matchOpen = $state(false);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let matchRelease = $state<ReleaseView | null>(null);

  $effect(() => {
    const id = sourceId;
    const isOpen = open;
    const releaseId = focusReleaseId;
    untrack(() => {
      clearTimeout(searchTimer);
      detailsRequests.invalidate();
      releaseRequests.invalidate();
      details = null;
      releases = [];
      total = 0;
      detailsError = '';
      releasesError = '';
      loadingReleases = false;
      // A retry still running when the dialog closes belongs to that opening: the next starts clean.
      detailsRetrying = false;
      releasesRetrying = false;
      if (isOpen && id) {
        filterStatus = 'all';
        search = '';
        page = 1;
        loadDetails(id);
        if (releaseId) focusRelease(id, releaseId);
        else loadReleases(id);
      }
    });
  });

  async function focusRelease(id: string, releaseId: string) {
    const ticket = releaseRequests.begin();
    loadingReleases = true;
    const result = await releaseRequests.settle(ticket, getRelease(releaseId));
    if (result.kind === 'stale') return;
    if (result.kind === 'error') toast(sourceErrorText(result.error, msg('modals.sourceDetailsReleaseLoadFailed')), 'danger');
    else if (result.value) search = result.value.release.rawTitle;
    loadReleases(id);
  }

  // A failure stays on screen, its Retry busy, until the answer replaces it.
  async function loadDetails(id: string) {
    const ticket = detailsRequests.begin();
    const result = await detailsRequests.settle(ticket, getSourceDetails(id));
    if (result.kind === 'stale') return;
    if (result.kind === 'error') {
      details = null;
      detailsError = sourceErrorText(result.error, msg('modals.sourceDetailsLoadFailed'));
    } else {
      details = result.value;
      detailsError = '';
    }
  }

  async function loadReleases(id: string) {
    const ticket = releaseRequests.begin();
    loadingReleases = true;
    const result = await releaseRequests.settle(ticket, queryReleases({ sourceId: id, search, status: filterStatus, page, pageSize }));
    if (result.kind === 'stale') return;
    if (result.kind === 'error') {
      releases = [];
      total = 0;
      releasesError = sourceErrorText(result.error, msg('modals.sourceDetailsReleasesFailed'));
    } else {
      releases = result.value.items;
      total = result.value.total;
      releasesError = '';
    }
    loadingReleases = false;
  }

  function reloadAll() {
    if (!sourceId) return;
    loadDetails(sourceId);
    loadReleases(sourceId);
  }

  let detailsRetrying = $state(false);
  let releasesRetrying = $state(false);
  let filtersEl = $state<HTMLElement>();
  let tableEl = $state<HTMLElement>();

  // The notices' retries load again; a failure stays on screen until the answer. On success the
  // focus goes to the status filter after the details, or to the first release the list brought.
  async function retryDetails(): Promise<boolean> {
    if (!sourceId) return false;
    await loadDetails(sourceId);
    return detailsError === '';
  }

  async function retryReleases(): Promise<boolean> {
    if (!sourceId) return false;
    await loadReleases(sourceId);
    return releasesError === '';
  }

  function onFilterChange(status: string) {
    filterStatus = status;
    page = 1;
    if (sourceId) loadReleases(sourceId);
  }

  function onSearchInput() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      page = 1;
      if (sourceId) loadReleases(sourceId);
    }, 250);
  }

  let pagerEl = $state<HTMLElement>();

  // Whether the pressed pager button held the focus: only then is there a focus to hand on.
  const heldBy = (event: MouseEvent) => event.currentTarget instanceof HTMLElement && document.activeElement === event.currentTarget;

  function prevPage(event: MouseEvent) {
    if (page <= 1) return;
    const held = heldBy(event);
    page -= 1;
    void turnPage(held);
  }

  function nextPage(event: MouseEvent) {
    if (page * pageSize >= total) return;
    const held = heldBy(event);
    page += 1;
    void turnPage(held);
  }

  // The focus the pressed pager button held is handed on twice: at once, when the button reached its
  // end and turned disabled, and after the page's answer, which can disable it or take the pager away.
  async function turnPage(held: boolean) {
    const loaded = sourceId ? loadReleases(sourceId) : undefined;
    if (!held) return;
    await keepPagerFocus();
    await loaded;
    await keepPagerFocus();
  }

  // To the other pager button while there is one; when a failed page took the pager away, to the
  // failure's Retry.
  async function keepPagerFocus() {
    await tick();
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected && !(active instanceof HTMLButtonElement && active.disabled)) return;
    const other = [...(pagerEl?.querySelectorAll<HTMLButtonElement>('button') ?? [])].find((button) => !button.disabled);
    (other ?? tableEl?.querySelector<HTMLElement>('.notice button'))?.focus();
  }

  async function doRefresh() {
    if (!sourceId) return;
    refreshing = true;
    await refreshSource(sourceId);
    refreshing = false;
    reloadAll();
  }

  async function doToggle() {
    if (!source) return;
    await toggleSource(source.id, !source.enabled);
  }

  function doRemove() {
    const current = source;
    if (!current) return;
    pending = {
      prompt: removeSourcePrompt(current.name),
      run: async () => {
        await removeSource(current.id);
        open = false;
      },
    };
  }

  function openMatch(view: ReleaseView) {
    matchRelease = view;
    matchOpen = true;
  }

  function matchLabel(view: ReleaseView) {
    if (view.release.matchStatus === 'matched') return { kind: 'success' as const, label: msg('modals.sourceDetailsStatusMatched') };
    if (view.release.matchStatus === 'review') return { kind: 'warning' as const, label: msg('modals.sourceDetailsStatusReview') };
    return { kind: 'neutral' as const, label: msg('modals.sourceDetailsStatusUnmatched') };
  }

  const from = $derived(total === 0 ? 0 : (page - 1) * pageSize + 1);
  const to = $derived(Math.min(page * pageSize, total));
</script>

<Modal bind:open title={source?.name ?? msg('modals.sourceDetailsTitle')} width="92rem">
  {#if source}
    <div class="sections">
      <section class="head">
        <div class="head-meta">
          <span class="url" title={sourceLocation(source)}>{sourceLocation(source)}</span>
          <div class="head-badges">
            <StatusBadge kind={statusBadgeInfo(source.status)?.kind ?? 'neutral'} label={statusBadgeInfo(source.status)?.label ?? source.status} />
            <span class="updated">{msg('modals.sourceDetailsUpdatedAt', { date: relativeDate(source.lastUpdatedAt) })}</span>
          </div>
          {#if source.lastError}
            <Notice tone="danger" compact announce={false}>{sourceErrorText(source.lastError)}</Notice>
          {/if}
        </div>
        <div class="head-actions">
          <Button size="sm" busy={refreshing || source.status === 'updating'} onclick={doRefresh}>
            {source.status === 'updating' ? msg('modals.sourceDetailsRefreshing') : msg('common.refresh')}
          </Button>
          <Button size="sm" onclick={doToggle}>{source.enabled ? msg('modals.sourceDetailsDisable') : msg('modals.sourceDetailsEnable')}</Button>
          <Button size="sm" variant="danger" onclick={doRemove}>{msg('common.delete')}</Button>
        </div>
      </section>

      {#if detailsError || detailsRetrying}
        <Notice tone="danger" compact bind:retrying={detailsRetrying} retry={{ label: msg('common.retry'), run: retryDetails, within: () => filtersEl }}>
          {detailsError}
        </Notice>
      {:else if details}
        <section class="counters">
          <div class="counter"><span class="counter-label">{msg('modals.sourceDetailsCounterTotal')}</span><span class="counter-value">{details.total}</span></div>
          <div class="counter"><span class="counter-label">{msg('modals.sourceDetailsCounterAvailable')}</span><span class="counter-value">{details.available}</span></div>
          <div class="counter"><span class="counter-label">{msg('modals.sourceDetailsCounterRemoved')}</span><span class="counter-value">{details.removed}</span></div>
          <div class="counter"><span class="counter-label">{msg('modals.sourceDetailsCounterNew')}</span><span class="counter-value">{details.new}</span></div>
          <div class="counter"><span class="counter-label">{msg('modals.sourceDetailsCounterIgnored')}</span><span class="counter-value">{details.ignored}</span></div>
        </section>
      {/if}

      <section class="filters" bind:this={filtersEl}>
        <Select options={statusOptions} value={filterStatus} width="18rem" label={msg('modals.sourceDetailsColStatus')} onchange={onFilterChange} />
        <SearchInput bind:value={search} label={msg('modals.sourceDetailsSearchPlaceholder')} placeholder={msg('modals.sourceDetailsSearchPlaceholder')} oninput={onSearchInput} />
      </section>

      <section class="table" bind:this={tableEl}>
        <div class="thead">
          <span class="th">{msg('modals.sourceDetailsColRawTitle')}</span>
          <span class="th">{msg('modals.sourceDetailsColGame')}</span>
          <span class="th">{msg('modals.sourceDetailsColVersion')}</span>
          <span class="th">{msg('modals.sourceDetailsColSize')}</span>
          <span class="th">{msg('modals.sourceDetailsColUploaded')}</span>
          <span class="th">{msg('modals.sourceDetailsColStatus')}</span>
        </div>
        {#if releasesError || releasesRetrying}
          <div class="list-note">
            <Notice tone="danger" compact bind:retrying={releasesRetrying} retry={{ label: msg('common.retry'), run: retryReleases, within: () => tableEl }}>
              {releasesError}
            </Notice>
          </div>
        {:else if loadingReleases}
          <div class="list-note loading"><Spinner size="sm" label={msg('modals.sourceDetailsLoading')} /></div>
        {:else if releases.length === 0}
          <div class="list-note">
            <EmptyState size="inline" align="center" title={msg('modals.sourceDetailsNoReleases')} />
          </div>
        {:else}
          {#each releases as view (view.release.id)}
            {@const badge = matchLabel(view)}
            <button type="button" class="row clickable" onclick={() => openMatch(view)}>
              <span class="cell title" title={view.release.rawTitle}>{view.release.rawTitle}</span>
              <span class="cell">{view.gameTitle || '—'}</span>
              <span class="cell">{view.release.edition || view.release.version || '—'}</span>
              <span class="cell nums">{bytesSize(view.release.size)}</span>
              <span class="cell">{relativeDate(view.release.uploadedAt)}</span>
              <span class="cell status">
                {#if view.release.availability === 'removed'}
                  <StatusBadge kind="danger" label={msg('modals.sourceDetailsUnavailable')} />
                {:else}
                  <StatusBadge kind={badge.kind} label={badge.label} />
                  {#if view.release.matchStatus === 'matched'}
                    <span class="pct">{Math.round(view.release.matchConfidence * 100)}%</span>
                  {/if}
                {/if}
                {#if view.release.new}
                  <StatusBadge kind="accent" label={msg('modals.sourceDetailsNewBadge')} dot={false} />
                {/if}
              </span>
            </button>
          {/each}
        {/if}
        {#if !releasesError || page > 1}
          <div class="tfoot">
            {#if !releasesError}
              <span class="range">{msg('modals.sourceDetailsRange', { from, to, total })}</span>
            {/if}
            <div class="pager" bind:this={pagerEl}>
              <IconButton size="sm" variant="secondary" label={msg('ui.previous')} disabled={page <= 1} onclick={prevPage}>
                <ChevronLeft size="1.5rem" strokeWidth={1.8} />
              </IconButton>
              <IconButton size="sm" variant="secondary" label={msg('ui.next')} disabled={page * pageSize >= total} onclick={nextPage}>
                <ChevronRight size="1.5rem" strokeWidth={1.8} />
              </IconButton>
            </div>
          </div>
        {/if}
      </section>
    </div>
  {/if}
</Modal>

<ReleaseMatchModal bind:open={matchOpen} release={matchRelease} onchanged={reloadAll} />
{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} errorText={(err) => sourceErrorText(err)} />
{/if}

<style>
  .sections {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .head-meta {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
    min-width: 0;
  }

  .url {
    font-size: var(--font-sm);
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .head-badges {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .updated {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .head-actions {
    display: flex;
    gap: 0.8rem;
    flex-shrink: 0;
  }

  .counters {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3) var(--space-8);
  }

  .counter {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .counter-value {
    font-size: var(--font-lg);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .counter-label {
    font-size: 1.2rem;
    color: var(--text-3);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .filters :global(.search) {
    flex: 1;
  }

  .table {
    background: var(--surface);
    border-radius: var(--radius-md);
    overflow: hidden;
  }

  .thead,
  .row {
    display: grid;
    grid-template-columns: minmax(16rem, 1.6fr) minmax(12rem, 1fr) 9rem 8rem 10rem 16rem;
    align-items: center;
    gap: var(--space-3);
    padding: 0 var(--space-4);
  }

  .thead {
    height: 4rem;
    border-bottom: 1px solid var(--border);
  }

  .th {
    font-size: 1.2rem;
    font-weight: 500;
    color: var(--text-3);
  }

  .row {
    width: 100%;
    height: 4.6rem;
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row.clickable {
    cursor: pointer;
  }

  .row.clickable:hover {
    background: var(--hover);
  }

  /* Inside the table: the shared ring is drawn inward, where the table does not clip it. */
  .row:focus-visible {
    outline-offset: -2px;
  }

  .row:disabled {
    cursor: default;
  }

  .cell {
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cell.title {
    color: var(--text);
  }

  .cell.nums {
    font-variant-numeric: tabular-nums;
  }

  .cell.status {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    overflow: visible;
  }

  .pct {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .list-note {
    padding: var(--space-6);
    text-align: center;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .list-note.loading {
    display: flex;
    justify-content: center;
  }

  .tfoot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 1.2rem var(--space-4);
    border-top: 1px solid var(--border);
  }

  .range {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .pager {
    display: flex;
    margin-left: auto;
    gap: 0.6rem;
  }
</style>
