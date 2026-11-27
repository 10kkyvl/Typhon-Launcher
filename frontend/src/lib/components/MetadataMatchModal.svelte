<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import {
    applyMetadataMatch,
    findMetadataCandidates,
    searchMetadataCandidates,
    type MetadataCandidate,
    type MetadataView,
  } from '../services/metadata';
  import { REASONS, metadataErrorText } from '../metadata/metadataErrors';
  import { toast } from '../stores/toasts';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import OptionList from './OptionList.svelte';
  import SearchInput from './SearchInput.svelte';
  import Spinner from './Spinner.svelte';
  import { errorCode, msg } from '../i18n';
  import { focusHandOff } from '../utils/focusHandOff';

  /**
   * The game's match in IGDB: the candidates Go found for it, and a search by name. Each list is an
   * option list whose choice is acted on at once (applied, or in change mode asked about first), so the
   * arrows only walk it and Enter, Space or a press choose. A failed look-up or search is said with a
   * Retry, never as "none found"; an answer for an earlier opening or another game paints nothing.
   */
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
  const candidatesId = `${uid}-candidates`;
  const manualSearchId = `${uid}-manual-search`;
  const failureId = `${uid}-failure`;

  type Row = { id: string; title: string; candidate: MetadataCandidate };
  const rows = (list: MetadataCandidate[]): Row[] => list.map((candidate) => ({ id: candidate.providerId, title: candidate.title, candidate }));

  let candidates = $state<MetadataCandidate[]>([]);
  let loadingCandidates = $state(false);
  let candidatesFailure = $state('');
  let candidatesRetrying = $state(false);
  let query = $state('');
  // The query the last search ran for, and what it answered: the words of "nothing found" name it.
  let searched = $state('');
  let searchResults = $state<MetadataCandidate[]>([]);
  let searching = $state(false);
  let searchFailure = $state('');
  let searchRetrying = $state(false);
  let applying = $state(false);
  // The row being applied: its mark shows which while the list is held.
  let applyingId = $state('');
  // Why the match was refused, in the launcher's words for the code.
  let failure = $state('');
  let pending = $state<MetadataCandidate | null>(null);
  let searchToken = 0;
  let candidatesToken = 0;
  // One per opening and game: an apply answered after the dialog closed or turned to another game is
  // that game's, not the one on screen. A destroyed dialog (the page showed a missing game) hears
  // nothing either: its bound `open` may already be the next game's dialog.
  let opening = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    const isOpen = open;
    const id = gameId;
    untrack(() => {
      opening += 1;
      // A search typed or sent in an earlier opening, or for another game, is dropped with it.
      clearTimeout(searchTimer);
      searchToken += 1;
      searching = false;
      query = '';
      searched = '';
      searchResults = [];
      searchFailure = '';
      candidates = [];
      candidatesFailure = '';
      pending = null;
      applying = false;
      applyingId = '';
      failure = '';
      candidatesToken += 1;
      loadingCandidates = false;
      if (isOpen && id) void loadCandidates(id);
    });
  });

  // Only the answer for the game the dialog is open for lands: a late one for the game before would
  // offer, and apply, another game's matches.
  async function loadCandidates(id: string): Promise<boolean | undefined> {
    const token = ++candidatesToken;
    loadingCandidates = true;
    try {
      const found = await findMetadataCandidates(id);
      if (token !== candidatesToken) return undefined;
      candidates = found;
      candidatesFailure = '';
      return true;
    } catch (err) {
      if (token !== candidatesToken) return undefined;
      candidates = [];
      candidatesFailure = metadataErrorText(err);
      return false;
    } finally {
      if (token === candidatesToken) loadingCandidates = false;
    }
  }

  async function search(value: string): Promise<boolean | undefined> {
    const token = ++searchToken;
    searching = true;
    try {
      const result = await searchMetadataCandidates(value);
      if (token !== searchToken) return undefined;
      searchResults = result;
      searched = value;
      searchFailure = '';
      return true;
    } catch (err) {
      if (token !== searchToken) return undefined;
      searchResults = [];
      searched = '';
      searchFailure = metadataErrorText(err);
      return false;
    } finally {
      if (token === searchToken) searching = false;
    }
  }

  function onQueryInput() {
    clearTimeout(searchTimer);
    const value = query.trim();
    if (!value) {
      searchToken += 1;
      searchResults = [];
      searched = '';
      searchFailure = '';
      searching = false;
      return;
    }
    searchTimer = setTimeout(() => void search(value), 300);
  }

  let confirmText = $state<HTMLElement>();
  let sectionsEl = $state<HTMLElement>();
  // The row that asked the question: the focus goes back to it when the list comes back.
  let pickedFrom: { list: 'found' | 'search'; id: string } | null = null;

  // In change mode a row swaps the list for a question: focus the row held goes into the question.
  function pick(candidate: MetadataCandidate, list: 'found' | 'search') {
    if (applying) return;
    failure = '';
    if (mode === 'change') {
      const hand = focusHandOff();
      pending = candidate;
      pickedFrom = { list, id: candidate.providerId };
      void hand(() => confirmText);
      return;
    }
    void apply(candidate, pressHand());
  }

  // The focus the press holds goes to the words of a refusal; a press that held none moves nothing.
  function pressHand() {
    const at = document.activeElement;
    return focusHandOff(() => at);
  }

  function cancelPending() {
    const hand = focusHandOff();
    pending = null;
    void hand(pickedRow);
  }

  function pickedRow(): HTMLElement | undefined {
    const from = pickedFrom;
    if (!from) return undefined;
    const list = from.list === 'found' ? candidates : searchResults;
    const at = list.findIndex((candidate) => candidate.providerId === from.id);
    return sectionsEl?.querySelectorAll<HTMLElement>(`[data-list="${from.list}"] [role="radio"]`)[at];
  }

  // `hand`: taken at the press; a refusal brings the list back with the words of it, which take the
  // focus the press held.
  // The match was made, or refused, for the game it was pressed for even when the dialog has moved
  // on: a toast names that game, and the dialog and the page now on screen are left as they are.
  async function apply(candidate: MetadataCandidate, hand: ReturnType<typeof focusHandOff>) {
    if (applying) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const title = gameTitle;
    applying = true;
    applyingId = candidate.providerId;
    failure = '';
    let view: MetadataView;
    try {
      view = await applyMetadataMatch(gameId, candidate.providerId);
    } catch (err) {
      if (!heard()) {
        toast(refusedFor(title, err), 'danger');
        return;
      }
      failure = metadataErrorText(err);
      settle();
      await hand(() => document.getElementById(failureId));
      return;
    }
    toast(msg('modals.metadataMatchApplied', { title: view.game.title }), 'success');
    if (!heard()) return;
    settle();
    open = false;
    onapplied?.(view);
  }

  function refusedFor(title: string, err: unknown): string {
    const reason = REASONS[errorCode(err)];
    return reason ? msg('modals.metadataMatchFailedForReason', { title, reason: msg(reason) }) : msg('modals.metadataMatchFailedFor', { title });
  }

  function settle() {
    applying = false;
    applyingId = '';
    pending = null;
  }

  function confidenceLabel(confidence: number) {
    if (confidence >= 0.85) return msg('modals.metadataMatchConfidenceExact');
    if (confidence >= 0.6) return msg('modals.metadataMatchConfidenceLikely');
    return msg('modals.metadataMatchConfidencePossible');
  }

  const heldReason = $derived(applying ? msg('modals.actionRunning') : '');
</script>

{#snippet match(row: Row)}
  {@const candidate = row.candidate}
  <span class="candidate">
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
  </span>
{/snippet}

<Modal dismissible={!applying} busy={applying} bind:open title={mode === 'change' ? msg('modals.metadataMatchChangeTitle') : msg('modals.metadataMatchFindTitle')} width="56rem">
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
      {#if failure}
        <Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice>
      {/if}
      <section class="block">
        <h4 id={candidatesId}>{msg('modals.metadataMatchCandidates')}</h4>
        {#if loadingCandidates && !candidatesRetrying}
          <div class="looking">
            <Spinner size="sm" label={msg('modals.metadataMatchSearchingCandidates')} />
            <p class="looking-text">{msg('modals.metadataMatchSearchingCandidatesDesc', { gameTitle })}</p>
          </div>
        {:else if candidatesFailure || candidatesRetrying}
          <Notice tone="danger" compact bind:retrying={candidatesRetrying} retry={{ label: msg('common.retry'), run: () => loadCandidates(gameId) }}>
            {candidatesFailure}
          </Notice>
        {:else if candidates.length === 0}
          <EmptyState
            level={5}
            title={msg('modals.metadataMatchNoCandidatesTitle')}
            description={msg('modals.metadataMatchNoCandidatesDesc')}
          />
        {:else}
          <div class="candidates" data-list="found">
            <OptionList
              labelledby={candidatesId}
              options={rows(candidates)}
              value={applyingId}
              activation="manual"
              disabled={applying}
              onchange={(id) => {
                const chosen = candidates.find((candidate) => candidate.providerId === id);
                if (chosen) pick(chosen, 'found');
              }}
              row={match}
            />
          </div>
        {/if}
      </section>

      <section class="block">
        <h4 id={manualSearchId}>{msg('modals.metadataMatchManualSearch')}</h4>
        <SearchInput bind:value={query} labelledby={manualSearchId} placeholder={msg('modals.metadataMatchNamePlaceholder')} loading={searching} oninput={onQueryInput} />
        {#if searchFailure || searchRetrying}
          <div class="after-search">
            <Notice tone="danger" compact bind:retrying={searchRetrying} retry={{ label: msg('common.retry'), run: () => search(query.trim()) }}>
              {searchFailure}
            </Notice>
          </div>
        {:else if searchResults.length > 0}
          <div class="candidates" data-list="search">
            <OptionList
              labelledby={manualSearchId}
              options={rows(searchResults)}
              value={applyingId}
              activation="manual"
              disabled={applying}
              onchange={(id) => {
                const chosen = searchResults.find((candidate) => candidate.providerId === id);
                if (chosen) pick(chosen, 'search');
              }}
              row={match}
            />
          </div>
        {:else if searched && !searching}
          <p class="nothing" role="status">{msg('modals.metadataMatchSearchEmpty', { query: searched })}</p>
        {/if}
      </section>
    </div>
  {/if}

  {#snippet footer()}
    {#if pending}
      <Button disabled={applying} reason={heldReason} onclick={cancelPending}>{msg('common.cancel')}</Button>
      <Button variant="primary" busy={applying} onclick={() => pending && apply(pending, pressHand())}>
        {applying ? msg('modals.metadataMatchApplying') : msg('modals.metadataMatchReplace')}
      </Button>
    {:else}
      <Button disabled={applying} reason={heldReason} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
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
    font-size: var(--font-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
    margin-bottom: var(--space-3);
  }

  .candidates {
    max-height: 26rem;
    overflow-y: auto;
    padding: var(--space-1);
    margin-top: 0.8rem;
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .after-search {
    margin-top: 0.8rem;
  }

  .candidate {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
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
    overflow-wrap: anywhere;
  }

  .year {
    color: var(--text-3);
    margin-left: 0.4rem;
  }

  .candidate-developer {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow-wrap: anywhere;
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

  .looking-text,
  .nothing {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .nothing {
    margin-top: 0.8rem;
  }

  .confirm-text {
    font-size: var(--font-md);
    line-height: 1.55;
    color: var(--text-2);
  }
</style>
