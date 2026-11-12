<script lang="ts">
  import { untrack } from 'svelte';
  import { Check } from '@lucide/svelte';
  import {
    confirmMatch,
    getMatchCandidates,
    ignoreRelease,
    searchGames,
    type CatalogGame,
    type MatchCandidate,
    type ReleaseView,
  } from '../services/sources';
  import { errorMessage } from '../stores/sources';
  import { metadataErrorText } from '../metadata/metadataErrors';
  import { toast } from '../stores/toasts';
  import { candidateGroups } from '../game/similar';
  import { bytesSize, relativeDate } from '../utils/format';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import SearchInput from './SearchInput.svelte';
  import Spinner from './Spinner.svelte';
  import Tooltip from './Tooltip.svelte';
  import { msg } from '../i18n';
  import { nextIndex } from '../utils/roving';

  let {
    open = $bindable(false),
    release,
    onchanged,
  }: {
    open?: boolean;
    release: ReleaseView | null;
    onchanged?: () => void;
  } = $props();
  const uid = $props.id();

  let candidates = $state<MatchCandidate[]>([]);
  const ordered = $derived.by(() => {
    const groups = candidateGroups(candidates);
    return [...groups.server, ...groups.local];
  });
  let loadingCandidates = $state(false);
  let selectedGameId = $state<string | null>(null);
  let query = $state('');
  let searchResults = $state<CatalogGame[]>([]);
  let searching = $state(false);
  let searchError = $state('');
  // The footer action running: its button stays focused and busy, the other waits.
  let submitting = $state<'' | 'confirm' | 'ignore'>('');
  let searchToken = 0;
  let candidatesToken = 0;
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    const isOpen = open;
    const current = release;
    untrack(() => {
      clearTimeout(searchTimer);
      searchToken += 1;
      candidatesToken += 1;
      loadingCandidates = false;
      searching = false;
      searchError = '';
      selectedGameId = null;
      query = '';
      searchResults = [];
      candidates = [];
      if (isOpen && current) loadCandidates(current.release.id);
    });
  });

  // Only the answer for the release the dialog is open for lands: a late one for the release before
  // would offer, and save, another release's games.
  async function loadCandidates(releaseId: string) {
    const token = ++candidatesToken;
    loadingCandidates = true;
    try {
      const found = await getMatchCandidates(releaseId);
      if (token === candidatesToken) candidates = found;
    } catch (err) {
      if (token === candidatesToken) toast(errorMessage(err), 'danger');
    } finally {
      if (token === candidatesToken) loadingCandidates = false;
    }
  }

  // A failed search stays on screen, its Retry busy, until the next answer replaces it.
  function onQueryInput() {
    clearTimeout(searchTimer);
    const token = ++searchToken;
    searching = false;
    const value = query.trim();
    if (!value) {
      searchResults = [];
      searchError = '';
      return;
    }
    searchTimer = setTimeout(() => void runSearch(value, token), 250);
  }

  async function runSearch(value: string, token: number) {
    searching = true;
    try {
      const result = await searchGames(value);
      if (token === searchToken) {
        searchResults = result;
        searchError = '';
      }
    } catch (err) {
      if (token === searchToken) {
        searchResults = [];
        searchError = errorMessage(err);
      }
    } finally {
      if (token === searchToken) searching = false;
    }
  }

  let searchRetrying = $state(false);
  let searchResultsEl = $state<HTMLElement>();
  let searchBlock = $state<HTMLElement>();

  // The notice's retry asks again at once, not after the typing pause; the failure stays on screen
  // until the answer. On success the focus goes to the results' Tab stop, else to the search field.
  async function retrySearch(): Promise<boolean> {
    clearTimeout(searchTimer);
    const value = query.trim();
    if (!value) {
      searchError = '';
      return true;
    }
    await runSearch(value, ++searchToken);
    return searchError === '';
  }

  async function confirm() {
    if (!release || !selectedGameId) return;
    submitting = 'confirm';
    try {
      await confirmMatch(release.release.id, selectedGameId);
      toast(msg('modals.releaseMatchSaved'), 'success');
      open = false;
      onchanged?.();
    } catch (err) {
      toast(metadataErrorText(err, errorMessage(err)), 'danger');
    } finally {
      submitting = '';
    }
  }

  async function ignore() {
    if (!release) return;
    submitting = 'ignore';
    try {
      await ignoreRelease(release.release.id, true);
      toast(msg('modals.releaseMatchIgnored'));
      open = false;
      onchanged?.();
    } catch (err) {
      toast(errorMessage(err), 'danger');
    } finally {
      submitting = '';
    }
  }

  // Each list of games is a radio group: the arrows move the focus and the choice together. A game that
  // is not in the catalog takes the focus, so its hint says why, but is not chosen.
  function moveChoice(event: KeyboardEvent, ids: string[], off: (index: number) => boolean) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const group = (event.currentTarget as HTMLElement).closest('[role="radiogroup"]');
    const radios = [...(group?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];
    const next = nextIndex(radios.indexOf(event.currentTarget as HTMLElement), ids.length, event.key, { orientation: 'vertical' });
    if (next === null) return;
    event.preventDefault();
    if (!off(next)) selectedGameId = ids[next];
    radios[next]?.focus();
  }

  // The one Tab stop of a list: its chosen game, else its first.
  function stopOf(ids: string[]): number {
    return Math.max(0, ids.findIndex((id) => id === selectedGameId));
  }

  const orderedIds = $derived(ordered.map((candidate) => candidate.gameId));
  const orderedOff = (index: number) => ordered[index]?.inCatalog === false;
  const orderedStop = $derived(stopOf(orderedIds));
  const resultIds = $derived(searchResults.map((game) => game.id));
  const resultStop = $derived(stopOf(resultIds));
</script>

<Modal dismissible={!submitting} bind:open title={msg('modals.releaseMatchTitle')} width="56rem">
  {#if release}
    <div class="sections">
      <section class="block">
        <div class="rows">
          <div class="row">
            <span class="key">{msg('modals.releaseMatchRawTitle')}</span>
            <span class="value">{release.release.rawTitle}</span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.releaseMatchNormalizedTitle')}</span>
            <span class="value">{release.release.normalizedTitle}</span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.releaseMatchSource')}</span>
            <span class="value">{release.sourceName}</span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.releaseMatchSize')}</span>
            <span class="value">{bytesSize(release.release.size)}</span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.releaseMatchUploaded')}</span>
            <span class="value">{relativeDate(release.release.uploadedAt)}</span>
          </div>
        </div>
      </section>

      <section class="block">
        <h4 id="{uid}-candidates">{msg('modals.releaseMatchCandidates')}</h4>
        {#if loadingCandidates}
          <Spinner size="sm" label={msg('modals.releaseMatchSearchingCandidates')} />
        {:else if candidates.length === 0}
          <EmptyState size="inline" title={msg('modals.releaseMatchNoCandidates')} />
        {:else}
          <div class="candidates" role="radiogroup" aria-labelledby="{uid}-candidates">
            {#each ordered as candidate, index (candidate.gameId)}
              {@const missing = candidate.inCatalog === false}
              <!-- Not disabled: a disabled button cannot take focus, and the reason would reach no keyboard. -->
              <Tooltip text={missing ? msg('modals.releaseMatchNotInCatalog') : ''}>
              <button
                type="button"
                class="candidate"
                role="radio"
                aria-checked={selectedGameId === candidate.gameId}
                aria-disabled={missing || undefined}
                tabindex={index === orderedStop ? 0 : -1}
                onclick={() => {
                  if (!missing) selectedGameId = candidate.gameId;
                }}
                onkeydown={(event) => moveChoice(event, orderedIds, orderedOff)}
              >
                <span class="radio" class:on={selectedGameId === candidate.gameId} aria-hidden="true">
                  {#if selectedGameId === candidate.gameId}<Check size="1.2rem" strokeWidth={2.6} />{/if}
                </span>
                <span class="candidate-main">
                  <span class="candidate-title" title={candidate.title}>{candidate.title}</span>
                  {#if candidate.platforms && candidate.platforms.length > 0}
                    {@const platforms = candidate.platforms.join(', ')}
                    <span class="candidate-platforms" title={platforms}>{platforms}</span>
                  {/if}
                </span>
                {#if candidate.year}<span class="year">{candidate.year}</span>{/if}
                {#if candidate.source === 'server'}
                  <span class="candidate-method">{msg('modals.releaseMatchFromCatalog')}</span>
                {/if}
                {#if candidate.source !== 'server'}
                  <span class="candidate-method">{candidate.method}</span>
                  <span class="candidate-score">{Math.round(candidate.score * 100)}%</span>
                {/if}
              </button>
              </Tooltip>
            {/each}
          </div>
        {/if}
      </section>

      <section class="block" bind:this={searchBlock}>
        <h4 id="{uid}-search">{msg('modals.releaseMatchManualSearch')}</h4>
        <div class="search-field">
          <SearchInput bind:value={query} labelledby="{uid}-search" placeholder={msg('modals.releaseMatchNamePlaceholder')} oninput={onQueryInput} />
        </div>
        {#if searchError || searchRetrying}
          <div>
            <Notice
              tone="danger"
              compact
              bind:retrying={searchRetrying}
              retry={{
                label: msg('common.retry'),
                run: retrySearch,
                focus: () => searchResultsEl?.querySelector<HTMLElement>('[role="radio"][tabindex="0"]'),
                within: () => searchBlock,
              }}
            >
              {searchError}
            </Notice>
          </div>
        {:else if searching}
          <Spinner size="sm" label={msg('modals.releaseMatchSearching')} />
        {:else if searchResults.length > 0}
          <div class="candidates" role="radiogroup" aria-labelledby="{uid}-search" bind:this={searchResultsEl}>
            {#each searchResults as game, index (game.id)}
              <button
                type="button"
                class="candidate"
                role="radio"
                aria-checked={selectedGameId === game.id}
                tabindex={index === resultStop ? 0 : -1}
                onclick={() => (selectedGameId = game.id)}
                onkeydown={(event) => moveChoice(event, resultIds, () => false)}
              >
                <span class="radio" class:on={selectedGameId === game.id} aria-hidden="true">
                  {#if selectedGameId === game.id}<Check size="1.2rem" strokeWidth={2.6} />{/if}
                </span>
                <span class="candidate-title" title={game.title}>{game.title}</span>
                {#if game.releaseYear}<span class="year">{game.releaseYear}</span>{/if}
              </button>
            {/each}
          </div>
        {/if}
      </section>
    </div>
  {/if}

  {#snippet footer()}
    <Button variant="danger" disabled={submitting === 'confirm'} busy={submitting === 'ignore'} onclick={ignore}>{msg('modals.releaseMatchIgnore')}</Button>
    <Button disabled={submitting !== ''} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button
      variant="primary"
      disabled={!selectedGameId || submitting === 'ignore'}
      busy={submitting === 'confirm'}
      onclick={confirm}
    >
      {msg('modals.releaseMatchConfirm')}
    </Button>
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

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.8rem 0;
    font-size: var(--font-sm);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .key {
    color: var(--text-3);
    flex-shrink: 0;
  }

  .value {
    min-width: 0;
    color: var(--text);
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .candidates {
    display: flex;
    flex-direction: column;
    max-height: 18rem;
    overflow-y: auto;
    padding: var(--space-1);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .candidate {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0.9rem 1rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .candidate:hover:not([aria-disabled='true']) {
    background: var(--hover);
  }

  .candidate[aria-disabled='true'] {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .candidate[aria-checked='true'] {
    background: var(--hover-strong);
  }

  /* Inside the scrolling list: the shared ring is drawn inward, where the list does not clip it. */
  .candidate:focus-visible {
    outline-offset: -2px;
  }

  .radio {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.8rem;
    height: 1.8rem;
    flex-shrink: 0;
    border-radius: 50%;
    border: 1px solid var(--border-strong);
    background: var(--surface-2);
    color: #fff;
  }

  .radio.on {
    background: var(--accent);
    border-color: var(--accent-bright);
  }

  .candidate-main {
    flex: 1 1 0;
    min-width: 12rem;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .candidate-platforms {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .candidate-title {
    min-width: 0;
    font-size: var(--font-sm);
    color: var(--text);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
  }

  .year {
    flex-shrink: 0;
    padding: 0.2rem 0.8rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    color: var(--text);
    font-size: var(--font-xs);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .candidate-method {
    font-size: 1.2rem;
    color: var(--text-3);
    flex-shrink: 0;
  }

  .candidate-score {
    font-size: var(--font-xs);
    font-weight: 600;
    color: var(--accent-text);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
    min-width: 4rem;
    text-align: right;
  }

  .search-field {
    margin-bottom: 0.8rem;
  }
</style>
