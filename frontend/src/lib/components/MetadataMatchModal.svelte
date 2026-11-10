<script lang="ts">
  import { untrack } from 'svelte';
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

  let candidates = $state<MetadataCandidate[]>([]);
  let loadingCandidates = $state(false);
  let query = $state('');
  let searchResults = $state<MetadataCandidate[]>([]);
  let searching = $state(false);
  let applying = $state(false);
  let pending = $state<MetadataCandidate | null>(null);
  let searchToken = 0;
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
      if (isOpen && id) loadCandidates(id);
    });
  });

  async function loadCandidates(id: string) {
    loadingCandidates = true;
    try {
      candidates = await findMetadataCandidates(id);
    } catch (err) {
      toast(metadataErrorText(err), 'danger');
      candidates = [];
    } finally {
      loadingCandidates = false;
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

  function pick(candidate: MetadataCandidate) {
    if (applying) return;
    if (mode === 'change') {
      pending = candidate;
      return;
    }
    apply(candidate);
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
  }

  function confidenceLabel(confidence: number) {
    if (confidence >= 0.85) return msg('modals.metadataMatchConfidenceExact');
    if (confidence >= 0.6) return msg('modals.metadataMatchConfidenceLikely');
    return msg('modals.metadataMatchConfidencePossible');
  }
</script>

<Modal bind:open title={mode === 'change' ? msg('modals.metadataMatchChangeTitle') : msg('modals.metadataMatchFindTitle')} width="56rem">
  {#if pending}
    <div class="confirm">
      <p class="confirm-text">
        {msg('modals.metadataMatchReplaceConfirm', {
          gameTitle,
          title: pending.title,
          year: pending.releaseYear ? ` (${pending.releaseYear})` : '',
        })}
      </p>
    </div>
  {:else}
    <div class="sections">
      <section class="block">
        <h4>{msg('modals.metadataMatchCandidates')}</h4>
        {#if loadingCandidates}
          <EmptyState title={msg('modals.metadataMatchSearchingCandidates')} description={msg('modals.metadataMatchSearchingCandidatesDesc', { gameTitle })} />
        {:else if candidates.length === 0}
          <EmptyState
            title={msg('modals.metadataMatchNoCandidatesTitle')}
            description={msg('modals.metadataMatchNoCandidatesDesc')}
          />
        {:else}
          <div class="candidates">
            {#each candidates as candidate (candidate.providerId)}
              <button class="candidate" disabled={applying} onclick={() => pick(candidate)}>
                <span class="thumb">
                  <Artwork src={candidate.thumb ?? ''} alt={candidate.title} radius="var(--radius-xs)" />
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
        <h4>{msg('modals.metadataMatchManualSearch')}</h4>
        <SearchInput bind:value={query} placeholder={msg('modals.metadataMatchNamePlaceholder')} loading={searching} oninput={onQueryInput} />
        {#if searchResults.length > 0}
          <div class="candidates">
            {#each searchResults as candidate (candidate.providerId)}
              <button class="candidate" disabled={applying} onclick={() => pick(candidate)}>
                <span class="thumb">
                  <Artwork src={candidate.thumb ?? ''} alt={candidate.title} radius="var(--radius-xs)" />
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
      <Button disabled={applying} onclick={() => (pending = null)}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={applying} onclick={() => pending && apply(pending)}>
        {applying ? msg('modals.metadataMatchApplying') : msg('modals.metadataMatchReplace')}
      </Button>
    {:else}
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
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

  .candidate:hover:not(:disabled) {
    background: var(--hover);
  }

  .candidate:disabled {
    opacity: 0.6;
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

  .confirm-text {
    font-size: var(--font-md);
    line-height: 1.55;
    color: var(--text-2);
  }
</style>
