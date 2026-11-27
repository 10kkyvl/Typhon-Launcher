<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import {
    confirmMatch,
    getMatchCandidates,
    ignoreRelease,
    searchGames,
    type CatalogGame,
    type MatchCandidate,
    type ReleaseView,
  } from '../services/sources';
  import { sourceErrorText } from '../sources/sourceErrors';
  import { metadataErrorText } from '../metadata/metadataErrors';
  import { toast } from '../stores/toasts';
  import { candidateGroups } from '../game/similar';
  import { bytesSize, relativeDate } from '../utils/format';
  import { focusHandOff } from '../utils/focusHandOff';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import OptionList from './OptionList.svelte';
  import type { OptionItem } from './optionList';
  import SearchInput from './SearchInput.svelte';
  import Spinner from './Spinner.svelte';
  import { msg, type MessageKey } from '../i18n';

  /**
   * Which catalog game a feed release is: the candidates Go found for it and a search by name, one
   * choice across both lists, saved by Confirm. The lists are option lists whose arrows only walk them
   * (a choice is a press, Space or Enter), held while a match or an ignore is on its way. Every failure
   * is said in the dialog in the launcher's words for its code, never Go's text; an answer for an
   * opening the dialog has left paints nothing here, and a refusal of one is a toast naming its release.
   */
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
  const failureId = `${uid}-failure`;

  let candidates = $state<MatchCandidate[]>([]);
  const ordered = $derived.by(() => {
    const groups = candidateGroups(candidates);
    return [...groups.server, ...groups.local];
  });
  let loadingCandidates = $state(false);
  let candidatesFailure = $state('');
  let candidatesRetrying = $state(false);
  let selectedGameId = $state<string | null>(null);
  let query = $state('');
  let searchResults = $state<CatalogGame[]>([]);
  let searching = $state(false);
  let searchError = $state('');
  let searchRetrying = $state(false);
  // The footer action running: its button stays focused and busy, the rest of the dialog is held.
  let submitting = $state<'' | 'confirm' | 'ignore'>('');
  // Why the last match or ignore was refused, in the launcher's words for its code.
  let failure = $state('');
  let searchToken = 0;
  // One per opening of the dialog and per release: an answer that arrives after its opening has
  // passed (closed, opened for another release, the dialog taken off the page) is not this one's.
  let opening = 0;
  let candidatesToken = 0;
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  // The reason by code: the sources' table, then the catalog's (a match looks the game up there), then
  // the dialog's own words for a code neither has.
  const reason = (err: unknown) => sourceErrorText(err, metadataErrorText(err, msg('modals.releaseMatchUnknownReason')));

  $effect(() => {
    const isOpen = open;
    const current = release;
    untrack(() => {
      clearTimeout(searchTimer);
      searchToken += 1;
      candidatesToken += 1;
      loadingCandidates = false;
      candidatesFailure = '';
      searching = false;
      searchError = '';
      // A retry or an action still running when the dialog closes belongs to that opening: the next
      // starts clean.
      opening += 1;
      searchRetrying = false;
      candidatesRetrying = false;
      submitting = '';
      failure = '';
      selectedGameId = null;
      query = '';
      searchResults = [];
      candidates = [];
      if (isOpen && current) void loadCandidates(current.release.id);
    });
  });

  onDestroy(() => {
    opening += 1;
    clearTimeout(searchTimer);
  });

  // Only the answer for the release the dialog is open for lands: a late one for the release before
  // would offer, and save, another release's games.
  async function loadCandidates(releaseId: string): Promise<boolean | undefined> {
    const token = ++candidatesToken;
    loadingCandidates = true;
    try {
      const found = await getMatchCandidates(releaseId);
      if (token !== candidatesToken) return undefined;
      candidates = found;
      candidatesFailure = '';
      return true;
    } catch (err) {
      if (token !== candidatesToken) return undefined;
      candidates = [];
      candidatesFailure = msg('modals.releaseMatchCandidatesFailed', { reason: reason(err) });
      return false;
    } finally {
      if (token === candidatesToken) loadingCandidates = false;
    }
  }

  async function retryCandidates(): Promise<boolean | undefined> {
    if (!release) return false;
    const mine = opening;
    const found = await loadCandidates(release.release.id);
    return mine === opening ? found : undefined;
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
        searchError = msg('modals.releaseMatchSearchFailed', { reason: reason(err) });
      }
    } finally {
      if (token === searchToken) searching = false;
    }
  }

  let searchResultsEl = $state<HTMLElement>();
  let searchBlock = $state<HTMLElement>();

  // The notice's retry asks again at once, not after the typing pause; the failure stays on screen
  // until the answer. On success the focus goes to the results' Tab stop, else to the search field.
  async function retrySearch(): Promise<boolean | undefined> {
    clearTimeout(searchTimer);
    const value = query.trim();
    if (!value) {
      searchError = '';
      return true;
    }
    const mine = opening;
    await runSearch(value, ++searchToken);
    return mine === opening ? searchError === '' : undefined;
  }

  type Action = {
    kind: 'confirm' | 'ignore';
    send: (releaseId: string) => Promise<void>;
    done: () => void;
    /** The dialog's words for a refusal while it is still on the release. */
    failedHere: MessageKey;
    /** A toast naming the release, for a refusal that answers after the dialog has left it. */
    failedFor: MessageKey;
  };

  // One request per press: while one runs its button is busy and the other is held (Button refuses
  // both). The focus the press holds goes to the words of a refusal; a press that held none moves nothing.
  async function act(action: Action) {
    const target = release;
    if (!target) return;
    const at = opening;
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    submitting = action.kind;
    failure = '';
    try {
      await action.send(target.release.id);
    } catch (err) {
      if (at !== opening) {
        toast(msg(action.failedFor, { title: target.release.rawTitle, reason: reason(err) }), 'danger');
        return;
      }
      submitting = '';
      failure = msg(action.failedHere, { reason: reason(err) });
      await hand(() => document.getElementById(failureId));
      return;
    }
    action.done();
    if (at !== opening) return;
    submitting = '';
    open = false;
    onchanged?.();
  }

  function confirm() {
    const gameId = selectedGameId;
    if (!gameId) return;
    void act({
      kind: 'confirm',
      send: (releaseId) => confirmMatch(releaseId, gameId),
      done: () => toast(msg('modals.releaseMatchSaved'), 'success'),
      failedHere: 'modals.releaseMatchConfirmFailed',
      failedFor: 'modals.releaseMatchFailedFor',
    });
  }

  function ignore() {
    void act({
      kind: 'ignore',
      send: (releaseId) => ignoreRelease(releaseId, true),
      done: () => toast(msg('modals.releaseMatchIgnored')),
      failedHere: 'modals.releaseMatchIgnoreFailed',
      failedFor: 'modals.releaseMatchIgnoreFailedFor',
    });
  }

  // How Go found a candidate (`catalog.Method`), in the player's words; a method this table does not
  // know yet gets the neutral word, never the code.
  const METHOD_WORDS: Record<string, MessageKey> = {
    external_id: 'modals.releaseMatchMethodExternalId',
    override: 'modals.releaseMatchMethodOverride',
    exact_title: 'modals.releaseMatchMethodExactTitle',
    alias: 'modals.releaseMatchMethodAlias',
    fuzzy: 'modals.releaseMatchMethodFuzzy',
    provisional: 'modals.releaseMatchMethodProvisional',
    server_title: 'modals.releaseMatchMethodServerTitle',
  };
  const methodWords = (method: string) => msg(Object.hasOwn(METHOD_WORDS, method) ? METHOD_WORDS[method] : 'modals.releaseMatchMethodUnknown');

  const candidateRows = $derived<OptionItem[]>(
    ordered.map((candidate) => ({
      id: candidate.gameId,
      title: candidate.title,
      note: candidate.platforms && candidate.platforms.length > 0 ? candidate.platforms.join(', ') : undefined,
      fact: [
        candidate.year ? String(candidate.year) : '',
        candidate.source === 'server' ? msg('modals.releaseMatchFromCatalog') : `${methodWords(candidate.method)} · ${Math.round(candidate.score * 100)}%`,
      ]
        .filter(Boolean)
        .join(' · '),
      disabledReason: candidate.inCatalog === false ? msg('modals.releaseMatchNotInCatalog') : undefined,
    })),
  );
  const resultRows = $derived<OptionItem[]>(
    searchResults.map((game) => ({ id: game.id, title: game.title, fact: game.releaseYear ? String(game.releaseYear) : undefined })),
  );
  const heldReason = $derived(submitting ? msg('modals.actionRunning') : '');
</script>

<Modal dismissible={!submitting} busy={submitting !== ''} bind:open title={msg('modals.releaseMatchTitle')} width="56rem">
  {#if release}
    <div class="sections">
      {#if failure}
        <Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice>
      {/if}
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
        {#if loadingCandidates && !candidatesRetrying}
          <Spinner size="sm" label={msg('modals.releaseMatchSearchingCandidates')} />
        {:else if candidatesFailure || candidatesRetrying}
          <Notice tone="danger" compact bind:retrying={candidatesRetrying} retry={{ label: msg('common.retry'), run: retryCandidates }}>
            {candidatesFailure}
          </Notice>
        {:else if candidates.length === 0}
          <EmptyState size="inline" title={msg('modals.releaseMatchNoCandidates')} />
        {:else}
          <div class="candidates">
            <OptionList
              labelledby="{uid}-candidates"
              options={candidateRows}
              value={selectedGameId ?? undefined}
              activation="manual"
              disabled={submitting !== ''}
              onchange={(id) => (selectedGameId = id)}
            />
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
          <div class="candidates" bind:this={searchResultsEl}>
            <OptionList
              labelledby="{uid}-search"
              options={resultRows}
              value={selectedGameId ?? undefined}
              activation="manual"
              disabled={submitting !== ''}
              onchange={(id) => (selectedGameId = id)}
            />
          </div>
        {/if}
      </section>
    </div>
  {/if}

  {#snippet footer()}
    <Button variant="danger" disabled={submitting === 'confirm'} reason={heldReason} busy={submitting === 'ignore'} onclick={ignore}>{msg('modals.releaseMatchIgnore')}</Button>
    <Button disabled={submitting !== ''} reason={heldReason} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button
      variant="primary"
      disabled={!selectedGameId || submitting === 'ignore'}
      reason={submitting === 'ignore' ? heldReason : ''}
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
    font-size: var(--font-xs);
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
    align-items: baseline;
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

  /* A long feed title wraps: the whole of it is on screen, not behind a hover. */
  .value {
    min-width: 0;
    color: var(--text);
    text-align: right;
    overflow-wrap: anywhere;
  }

  .candidates {
    max-height: 26rem;
    overflow-y: auto;
    padding: var(--space-1);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .search-field {
    margin-bottom: 0.8rem;
  }
</style>
