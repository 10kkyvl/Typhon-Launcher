<script lang="ts">
  import { sourceErrorText } from '../sources/sourceErrors';
  import { untrack } from 'svelte';
  import { ChevronLeft, ChevronRight } from '@lucide/svelte';
  import { getRelease, queryReleases, type ReleaseView } from '../services/sources';
  import { LatestRequestGate } from '../bigpicture/latestRequest';
  import { toast } from '../stores/toasts';
  import { relativeDate, bytesSize } from '../utils/format';
  import { focusHandOff } from '../utils/focusHandOff';
  import EmptyState from './EmptyState.svelte';
  import IconButton from './IconButton.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import SearchInput from './SearchInput.svelte';
  import Select from './Select.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import { msg } from '../i18n';

  /**
   * The releases of a source in its details dialog: the status filter, the search, the list and its
   * pager. It lives as long as the dialog is open and starts again for every source and every release
   * a link asks for; `shown` says whether the dialog shows it, as it does once the source is listed.
   */
  let {
    open,
    sourceId,
    focusReleaseId = null,
    shown,
    onopen,
  }: {
    /** Whether the dialog is open: a closing one drops what is still on its way at once. */
    open: boolean;
    sourceId: string | null;
    focusReleaseId?: string | null;
    shown: boolean;
    /** A release of the list was pressed. */
    onopen: (view: ReleaseView) => void;
  } = $props();

  const statusOptions = $derived([
    { id: 'all', label: msg('modals.sourceDetailsStatusAll') },
    { id: 'matched', label: msg('modals.sourceDetailsStatusMatched') },
    { id: 'review', label: msg('modals.sourceDetailsStatusReview') },
    { id: 'unmatched', label: msg('modals.sourceDetailsStatusUnmatched') },
    { id: 'removed', label: msg('modals.sourceDetailsStatusRemoved') },
    { id: 'new', label: msg('modals.sourceDetailsStatusNew') },
  ]);

  const releaseRequests = new LatestRequestGate();
  let releasesError = $state('');
  let filterStatus = $state('all');
  let search = $state('');
  let page = $state(1);
  const pageSize = 50;
  let releases = $state<ReleaseView[]>([]);
  let total = $state(0);
  let loadingReleases = $state(false);
  let searchTimer: ReturnType<typeof setTimeout> | undefined;
  // One per opening of the dialog: a retry that answers after its opening has passed says so.
  let opening = 0;
  let releasesRetrying = $state(false);
  let filtersEl = $state<HTMLElement>();
  let tableEl = $state<HTMLElement>();
  let pagerEl = $state<HTMLElement>();

  $effect(() => {
    const id = sourceId;
    const isOpen = open;
    const releaseId = focusReleaseId;
    untrack(() => {
      clearTimeout(searchTimer);
      releaseRequests.invalidate();
      releases = [];
      total = 0;
      releasesError = '';
      loadingReleases = false;
      // A retry still running when the dialog closes belongs to that opening: the next starts clean.
      opening += 1;
      releasesRetrying = false;
      if (isOpen && id) {
        filterStatus = 'all';
        search = '';
        page = 1;
        if (releaseId) focusRelease(id, releaseId);
        else loadReleases(id);
      }
    });
    // Gone with the dialog's content: an answer of this opening, and a search still waiting, change nothing.
    return () => {
      clearTimeout(searchTimer);
      releaseRequests.invalidate();
    };
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

  /** Reads the list again, as it stands: after a refresh or a match. */
  export function reload() {
    if (sourceId) loadReleases(sourceId);
  }

  /** The status filter and the search: where a Retry of the details that worked leaves the focus. */
  export function filters() {
    return filtersEl;
  }

  // The notice's retry loads again; a failure stays on screen until the answer. On success the focus
  // goes to the first release the list brought.
  async function retryReleases(): Promise<boolean | undefined> {
    if (!sourceId) return false;
    const mine = opening;
    await loadReleases(sourceId);
    return mine === opening ? releasesError === '' : undefined;
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

  function prevPage(event: MouseEvent) {
    if (page <= 1) return;
    const hand = pagerHandOff(event);
    page -= 1;
    void turnPage(hand);
  }

  function nextPage(event: MouseEvent) {
    if (page * pageSize >= total) return;
    const hand = pagerHandOff(event);
    page += 1;
    void turnPage(hand);
  }

  // A pager button at its end stays but turns disabled: focus still on it then counts as fallen.
  function pagerHandOff(event: MouseEvent) {
    const pressed = event.currentTarget instanceof HTMLButtonElement ? event.currentTarget : null;
    return focusHandOff(() => (pressed?.disabled ? pressed : null));
  }

  // Focus the pressed pager button held is handed on twice: at once, when the button reached its end,
  // and after the page's answer, which can disable it or take the pager away. It goes to the other
  // pager button while there is one; when a failed page took the pager away, to the failure's Retry.
  async function turnPage(hand: ReturnType<typeof focusHandOff>) {
    const loaded = sourceId ? loadReleases(sourceId) : undefined;
    const next = () =>
      [...(pagerEl?.querySelectorAll<HTMLButtonElement>('button') ?? [])].find((button) => !button.disabled) ??
      tableEl?.querySelector<HTMLElement>('.notice button');
    await hand(next);
    await loaded;
    await hand(next);
  }

  function matchLabel(view: ReleaseView) {
    if (view.release.matchStatus === 'matched') return { kind: 'success' as const, label: msg('modals.sourceDetailsStatusMatched') };
    if (view.release.matchStatus === 'review') return { kind: 'warning' as const, label: msg('modals.sourceDetailsStatusReview') };
    return { kind: 'neutral' as const, label: msg('modals.sourceDetailsStatusUnmatched') };
  }

  const from = $derived(total === 0 ? 0 : (page - 1) * pageSize + 1);
  const to = $derived(Math.min(page * pageSize, total));
</script>

{#if shown}
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
        <button type="button" class="row clickable" onclick={() => onopen(view)}>
          <span class="cell title">{view.release.rawTitle}</span>
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
{/if}

<style>
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
    font-size: var(--font-xs);
    font-weight: 500;
    color: var(--text-3);
  }

  .row {
    width: 100%;
    min-height: 4.6rem;
    padding-block: 0.8rem;
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

  /* A long release or game title wraps in its column: the row grows, nothing is cut. */
  .cell {
    min-width: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
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
