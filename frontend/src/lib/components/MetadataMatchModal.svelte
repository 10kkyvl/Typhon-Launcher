<script lang="ts">
  import { tick, untrack } from 'svelte';
  import {
    applyMetadataMatch,
    findMetadataCandidates,
    searchMetadataCandidates,
    type MetadataCandidate,
    type MetadataView,
  } from '../services/metadata';
  import { metadataErrorText } from '../metadata/metadataErrors';
  import { toast } from '../stores/toasts';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Modal from './Modal.svelte';
  import SearchInput from './SearchInput.svelte';
  import Spinner from './Spinner.svelte';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    gameId,
    gameTitle,
    mode = 'find',
    onapplied,
  }: {
    open?: boolean;
    gameId: string;
    gameTitle: string;
    mode?: 'find' | 'change';
    onapplied?: (view: MetadataView) => void;
  } = $props();

  const uid = $props.id();
  const manualSearchId = `${uid}-manual-search`;

  let candidates = $state<MetadataCandidate[]>([]);
  let loadingCandidates = $state(false);
  let query = $state('');
  let searchResults = $state<MetadataCandidate[]>([]);
  let searching = $state(false);
  let applying = $state(false);
  let pending = $state<MetadataCandidate | null>(null);
  let searchToken = 0;
  let candidatesToken = 0;
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    const isOpen = open;
    const id = gameId;
    untrack(() => {
      query = '';
      searchResults = [];
      candidates = [];
      pending = null;
      applying = false;
      candidatesToken += 1;
      loadingCandidates = false;
      if (isOpen && id) loadCandidates(id);
    });
  });

  // Only the answer for the game the dialog is open for lands: a late one for the game before would
  // offer, and apply, another game's matches.
  async function loadCandidates(id: string) {
    const token = ++candidatesToken;
    loadingCandidates = true;
    try {
      const found = await findMetadataCandidates(id);
      if (token === candidatesToken) candidates = found;
    } catch (err) {
      if (token !== candidatesToken) return;
      toast(metadataErrorText(err), 'danger');
      candidates = [];
    } finally {
      if (token === candidatesToken) loadingCandidates = false;
    }
  }

  function onQueryInput() {
    clearTimeout(searchTimer);
    const value = query.trim();
    if (!value) {
      searchResults = [];
      searching = false;
      return;
    }
    searchTimer = setTimeout(async () => {
      const token = ++searchToken;
      searching = true;
      try {
        const result = await searchMetadataCandidates(value);
        if (token === searchToken) searchResults = result;
      } catch (err) {
        if (token === searchToken) {
          searchResults = [];
          toast(metadataErrorText(err), 'danger');
        }
      } finally {
        if (token === searchToken) searching = false;
      }
    }, 300);
  }

  let confirmText = $state<HTMLElement>();
  let sectionsEl = $state<HTMLElement>();
  // The row that asked the question: the focus goes back to it when the list comes back.
  let pickedFrom: { list: 'found' | 'search'; id: string } | null = null;

  // In change mode a row swaps the list for a question: the focus goes into the question.
  async function pick(candidate: MetadataCandidate, list: 'found' | 'search') {
    if (applying) return;
    if (mode === 'change') {
      pending = candidate;
      pickedFrom = { list, id: candidate.providerId };
      await tick();
      confirmText?.focus();
      return;
    }
    apply(candidate);
  }

  function cancelPending() {
    pending = null;
    void backToRow();
  }

  async function backToRow() {
    await tick();
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected && !(active instanceof HTMLButtonElement && active.disabled)) return;
    const from = pickedFrom;
    if (!from) return;
    const rows = [...(sectionsEl?.querySelectorAll<HTMLElement>(`[data-list="${from.list}"] [data-provider]`) ?? [])];
    rows.find((row) => row.dataset.provider === from.id)?.focus();
  }

  async function apply(candidate: MetadataCandidate) {
    applying = true;
    try {
      const view = await applyMetadataMatch(gameId, candidate.providerId);
      toast(msg('modals.metadataMatchApplied', { title: view.game.title }), 'success');
      open = false;
      onapplied?.(view);
    } catch (err) {
      toast(metadataErrorText(err), 'danger');
    } finally {
      applying = false;
      pending = null;
    }
    if (open) await backToRow();
  }

  function confidenceLabel(confidence: number) {
    if (confidence >= 0.85) return msg('modals.metadataMatchConfidenceExact');
    if (confidence >= 0.6) return msg('modals.metadataMatchConfidenceLikely');
    return msg('modals.metadataMatchConfidencePossible');
  }
</script>

<Modal dismissible={!applying} bind:open title={mode === 'change' ? msg('modals.metadataMatchChangeTitle') : msg('modals.metadataMatchFindTitle')} width="56rem">
  {#if pending}
    <div class="confirm">
      <p class="confirm-text" tabindex="-1" bind:this={confirmText}>
        {msg('modals.metadataMatchReplaceConfirm', {
          gameTitle,
          title: pending.title,
          year: pending.releaseYear ? ` (${pending.releaseYear})` : '',
        })}
      </p>
    </div>
  {:else}
    <div class="sections" bind:this={sectionsEl}>
      <section class="block">
        <h4>{msg('modals.metadataMatchCandidates')}</h4>
        {#if loadingCandidates}
          <div class="looking">
            <Spinner size="sm" label={msg('modals.metadataMatchSearchingCandidates')} />
            <p class="looking-text">{msg('modals.metadataMatchSearchingCandidatesDesc', { gameTitle })}</p>
          </div>
        {:else if candidates.length === 0}
          <EmptyState level={5}
            title={msg('modals.metadataMatchNoCandidatesTitle')}
            description={msg('modals.metadataMatchNoCandidatesDesc')}
          />
        {:else}
          <div class="candidates" data-list="found">
            {#each candidates as candidate (candidate.providerId)}
              <button type="button" class="candidate" data-provider={candidate.providerId} aria-disabled={applying || undefined} onclick={() => pick(candidate, 'found')}>
                <span class="thumb">
                  <Artwork src={candidate.thumb ?? ''} alt="" label={candidate.title} radius="var(--radius-xs)" />
                </span>
                <span class="candidate-info">
                  <span class="candidate-title">
                    {candidate.title}
                    {#if candidate.releaseYear}<span class="year">({candidate.releaseYear})</span>{/if}
                  </span>
                  {#if candidate.developer}<span class="candidate-developer">{candidate.developer}</span>{/if}
                </span>
                <span class="candidate-confidence" class:high={candidate.confidence >= 0.85}>
                  {confidenceLabel(candidate.confidence)}
                </span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="block">
        <h4 id={manualSearchId}>{msg('modals.metadataMatchManualSearch')}</h4>
        <SearchInput bind:value={query} labelledby={manualSearchId} placeholder={msg('modals.metadataMatchNamePlaceholder')} loading={searching} oninput={onQueryInput} />
        {#if searchResults.length > 0}
          <div class="candidates" data-list="search">
            {#each searchResults as candidate (candidate.providerId)}
              <button type="button" class="candidate" data-provider={candidate.providerId} aria-disabled={applying || undefined} onclick={() => pick(candidate, 'search')}>
                <span class="thumb">
                  <Artwork src={candidate.thumb ?? ''} alt="" label={candidate.title} radius="var(--radius-xs)" />
                </span>
                <span class="candidate-info">
                  <span class="candidate-title">
                    {candidate.title}
                    {#if candidate.releaseYear}<span class="year">({candidate.releaseYear})</span>{/if}
                  </span>
                  {#if candidate.developer}<span class="candidate-developer">{candidate.developer}</span>{/if}
                </span>
                <span class="candidate-confidence" class:high={candidate.confidence >= 0.85}>
                  {confidenceLabel(candidate.confidence)}
                </span>
              </button>
            {/each}
          </div>
        {/if}
      </section>
    </div>
  {/if}

  {#snippet footer()}
    {#if pending}
      <Button disabled={applying} onclick={cancelPending}>{msg('common.cancel')}</Button>
      <Button variant="primary" busy={applying} onclick={() => pending && apply(pending)}>
        {applying ? msg('modals.metadataMatchApplying') : msg('modals.metadataMatchReplace')}
      </Button>
    {:else}
      <Button disabled={applying} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .sections {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .block h4 {
    font-size: 1.2rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
    margin-bottom: var(--space-3);
  }

  .candidates {
    display: flex;
    flex-direction: column;
    max-height: 26rem;
    overflow-y: auto;
    padding: var(--space-1);
    margin-top: 0.8rem;
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .candidate {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0.7rem 1rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .candidate:hover:not([aria-disabled='true']) {
    background: var(--hover);
  }

  .candidate[aria-disabled='true'] {
    opacity: 0.6;
    cursor: default;
  }

  /* Inside the scrolling list: the shared ring is drawn inward, where the list does not clip it. */
  .candidate:focus-visible {
    outline-offset: -2px;
  }

  .thumb {
    width: 3.6rem;
    height: 4.8rem;
    flex-shrink: 0;
    border-radius: var(--radius-xs);
    overflow: hidden;
  }

  .candidate-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .candidate-title {
    font-size: var(--font-sm);
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .year {
    color: var(--text-3);
    margin-left: 0.4rem;
  }

  .candidate-developer {
    font-size: 1.2rem;
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .candidate-confidence {
    font-size: var(--font-xs);
    font-weight: 500;
    color: var(--text-3);
    flex-shrink: 0;
    text-align: right;
    max-width: 14rem;
  }

  .candidate-confidence.high {
    color: var(--accent-text);
    font-weight: 600;
  }

  .looking {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .looking-text {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .confirm-text {
    font-size: var(--font-md);
    line-height: 1.55;
    color: var(--text-2);
  }
</style>
