<script lang="ts">
  import { sourceErrorText } from '../sources/sourceErrors';
  import { untrack } from 'svelte';
  import { getSourceDetails, sourceLocation, type ReleaseView, type SourceDetails } from '../services/sources';
  import { LatestRequestGate } from '../bigpicture/latestRequest';
  import { removeSourcePrompt, type ConfirmPrompt } from '../confirm/prompts';
  import { refresh as refreshSource, remove as removeSource, removeHeldReason, sources, toggle as toggleSource } from '../stores/sources';
  import { relativeDate } from '../utils/format';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import ReleaseMatchModal from './ReleaseMatchModal.svelte';
  import SourceReleases from './SourceReleases.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    sourceId,
    focusReleaseId = null,
  }: { open?: boolean; sourceId: string | null; focusReleaseId?: string | null } = $props();

  const source = $derived(sourceId ? $sources.find((s) => s.id === sourceId) : undefined);

  function statusBadgeInfo(status: string): { kind: 'success' | 'warning' | 'neutral'; label: string } {
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
        return { kind: 'neutral', label: msg('modals.sourceDetailsStatusUnknown') };
    }
  }

  const detailsRequests = new LatestRequestGate();
  let details = $state<SourceDetails | null>(null);
  let detailsError = $state('');
  let refreshing = $state(false);
  let toggling = $state(false);
  // One per opening of the dialog: a retry that answers after its opening has passed says so.
  let opening = 0;
  let releasesPart = $state<SourceReleases>();

  let matchOpen = $state(false);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let matchRelease = $state<ReleaseView | null>(null);

  $effect(() => {
    const id = sourceId;
    const isOpen = open;
    // A link to another release of the source opens the dialog afresh, its details included.
    void focusReleaseId;
    untrack(() => {
      detailsRequests.invalidate();
      details = null;
      detailsError = '';
      // A retry still running when the dialog closes belongs to that opening: the next starts clean.
      opening += 1;
      detailsRetrying = false;
      // An action of the opening before answers to it, not to this one.
      refreshing = false;
      toggling = false;
      if (isOpen && id) loadDetails(id);
    });
  });

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

  function reloadAll() {
    if (!sourceId) return;
    loadDetails(sourceId);
    releasesPart?.reload();
  }

  let detailsRetrying = $state(false);

  // The notice's retry loads again; a failure stays on screen until the answer. On success the focus
  // goes to the status filter after the details.
  async function retryDetails(): Promise<boolean | undefined> {
    if (!sourceId) return false;
    const mine = opening;
    await loadDetails(sourceId);
    return mine === opening ? detailsError === '' : undefined;
  }

  // Refresh and enable each send one request per press: the pressed button is busy, the other actions
  // wait with a reason. The store says how a refresh or a switch ended (a toast by code); an answer for
  // an opening the dialog has left changes nothing here.
  async function doRefresh() {
    if (!sourceId) return;
    const mine = opening;
    refreshing = true;
    await refreshSource(sourceId);
    if (mine !== opening) return;
    refreshing = false;
    reloadAll();
  }

  async function doToggle() {
    if (!source) return;
    const mine = opening;
    toggling = true;
    await toggleSource(source.id, !source.enabled);
    if (mine === opening) toggling = false;
  }

  // The question says what goes with the source (its releases, and updates for the games installed
  // from it) and what stays (the games).
  function doRemove() {
    const current = source;
    if (!current) return;
    pending = {
      prompt: { ...removeSourcePrompt(current.name), text: msg('modals.sourceDetailsRemoveText', { name: current.name }) },
      run: async () => {
        const mine = opening;
        await removeSource(current.id);
        if (mine === opening) open = false;
      },
    };
  }

  function openMatch(view: ReleaseView) {
    matchRelease = view;
    matchOpen = true;
  }

  const statusBadge = $derived(statusBadgeInfo(source?.status ?? ''));
  const acting = $derived(refreshing || toggling);
  const heldReason = $derived(acting ? msg('modals.actionRunning') : '');
  // A refresh started elsewhere ("Refresh all", the schedule) holds Delete as the dialog's own does.
  const removeHeld = $derived(heldReason || removeHeldReason(source));
</script>

<Modal bind:open title={source?.name ?? msg('modals.sourceDetailsTitle')} width="92rem">
  <!-- The releases live while the dialog is open, shown once the source is listed. -->
  <div class="sections">
    {#if source}
      <section class="head">
        <div class="head-meta">
          <span class="url">{sourceLocation(source)}</span>
          <div class="head-badges">
            <StatusBadge kind={statusBadge.kind} label={statusBadge.label} />
            <span class="updated">{msg('modals.sourceDetailsUpdatedAt', { date: relativeDate(source.lastUpdatedAt) })}</span>
          </div>
          {#if source.lastError}
            <Notice tone="danger" compact announce={false}>{sourceErrorText(source.lastError)}</Notice>
          {/if}
        </div>
        <div class="head-actions">
          <Button size="sm" busy={refreshing || source.status === 'updating'} disabled={toggling} reason={heldReason} onclick={doRefresh}>
            {source.status === 'updating' ? msg('modals.sourceDetailsRefreshing') : msg('common.refresh')}
          </Button>
          <Button size="sm" busy={toggling} disabled={refreshing} reason={heldReason} onclick={doToggle}>
            {source.enabled ? msg('modals.sourceDetailsDisable') : msg('modals.sourceDetailsEnable')}
          </Button>
          <Button size="sm" variant="danger" disabled={removeHeld !== ''} reason={removeHeld} onclick={doRemove}>{msg('common.delete')}</Button>
        </div>
      </section>

      {#if detailsError || detailsRetrying}
        <Notice tone="danger" compact bind:retrying={detailsRetrying} retry={{ label: msg('common.retry'), run: retryDetails, within: () => releasesPart?.filters() }}>
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
      {:else}
        <div class="counters-loading"><Spinner size="sm" label={msg('modals.sourceDetailsLoadingDetails')} /></div>
      {/if}
    {/if}

    <SourceReleases bind:this={releasesPart} {open} {sourceId} {focusReleaseId} shown={source !== undefined} onopen={openMatch} />
  </div>
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

  /* A long URL or path wraps: the whole of it is on screen, not behind a hover. */
  .url {
    font-size: var(--font-sm);
    color: var(--text-3);
    overflow-wrap: anywhere;
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

  .counters-loading {
    display: flex;
  }

  .counter-label {
    font-size: var(--font-xs);
    color: var(--text-3);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
</style>
