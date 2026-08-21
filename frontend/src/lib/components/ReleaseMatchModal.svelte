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
  import { toast } from '../stores/toasts';
  import { bytesSize, relativeDate } from '../utils/format';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  let {
    open = $bindable(false),
    release,
    onchanged,
  }: {
    open?: boolean;
    release: ReleaseView | null;
    onchanged?: () => void;
  } = $props();

  let candidates = $state<MatchCandidate[]>([]);
  let loadingCandidates = $state(false);
  let selectedGameId = $state<string | null>(null);
  let query = $state('');
  let searchResults = $state<CatalogGame[]>([]);
  let searching = $state(false);
  let submitting = $state(false);
  let searchToken = 0;
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    const isOpen = open;
    const current = release;
    untrack(() => {
      selectedGameId = null;
      query = '';
      searchResults = [];
      candidates = [];
      if (isOpen && current) loadCandidates(current.release.id);
    });
  });

  async function loadCandidates(releaseId: string) {
    loadingCandidates = true;
    try {
      candidates = await getMatchCandidates(releaseId);
    } catch (err) {
      toast(errorMessage(err), 'danger');
    } finally {
      loadingCandidates = false;
    }
  }

  function onQueryInput() {
    clearTimeout(searchTimer);
    const value = query.trim();
    if (!value) {
      searchResults = [];
      return;
    }
    searchTimer = setTimeout(async () => {
      const token = ++searchToken;
      searching = true;
      try {
        const result = await searchGames(value);
        if (token === searchToken) searchResults = result;
      } catch {
        if (token === searchToken) searchResults = [];
      } finally {
        if (token === searchToken) searching = false;
      }
    }, 250);
  }

  async function confirm() {
    if (!release || !selectedGameId) return;
    submitting = true;
    try {
      await confirmMatch(release.release.id, selectedGameId);
      toast('Сопоставление сохранено, похожие релизы будут сопоставляться автоматически', 'success');
      open = false;
      onchanged?.();
    } catch (err) {
      toast(errorMessage(err), 'danger');
    } finally {
      submitting = false;
    }
  }

  async function ignore() {
    if (!release) return;
    submitting = true;
    try {
      await ignoreRelease(release.release.id, true);
      toast('Релиз проигнорирован');
      open = false;
      onchanged?.();
    } catch (err) {
      toast(errorMessage(err), 'danger');
    } finally {
      submitting = false;
    }
  }
</script>

<Modal bind:open title="Сопоставление релиза" width="56rem">
  {#if release}
    <div class="sections">
      <section class="block">
        <div class="rows">
          <div class="row">
            <span class="key">Исходное название</span>
            <span class="value">{release.release.rawTitle}</span>
          </div>
          <div class="row">
            <span class="key">Нормализованное название</span>
            <span class="value">{release.release.normalizedTitle}</span>
          </div>
          <div class="row">
            <span class="key">Источник</span>
            <span class="value">{release.sourceName}</span>
          </div>
          <div class="row">
            <span class="key">Размер</span>
            <span class="value">{bytesSize(release.release.size)}</span>
          </div>
          <div class="row">
            <span class="key">Загружено</span>
            <span class="value">{relativeDate(release.release.uploadedAt)}</span>
          </div>
        </div>
      </section>

      <section class="block">
        <h4>Кандидаты</h4>
        {#if loadingCandidates}
          <p class="muted">Поиск кандидатов…</p>
        {:else if candidates.length === 0}
          <p class="muted">Автоматических кандидатов не найдено. Найдите игру вручную.</p>
        {:else}
          <div class="candidates">
            {#each candidates as candidate (candidate.gameId)}
              <button
                class="candidate"
                class:selected={selectedGameId === candidate.gameId}
                onclick={() => (selectedGameId = candidate.gameId)}
              >
                <span class="radio" class:on={selectedGameId === candidate.gameId}>
                  {#if selectedGameId === candidate.gameId}<Check size="1.2rem" strokeWidth={2.6} />{/if}
                </span>
                <span class="candidate-title">
                  {candidate.title}
                  {#if candidate.year}<span class="year">({candidate.year})</span>{/if}
                </span>
                <span class="candidate-method">{candidate.method}</span>
                <span class="candidate-score">{Math.round(candidate.score * 100)}%</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="block">
        <h4>Поиск вручную</h4>
        <input
          type="text"
          placeholder="Название игры"
          bind:value={query}
          oninput={onQueryInput}
        />
        {#if searching}
          <p class="muted">Поиск…</p>
        {:else if searchResults.length > 0}
          <div class="candidates">
            {#each searchResults as game (game.id)}
              <button
                class="candidate"
                class:selected={selectedGameId === game.id}
                onclick={() => (selectedGameId = game.id)}
              >
                <span class="radio" class:on={selectedGameId === game.id}>
                  {#if selectedGameId === game.id}<Check size="1.2rem" strokeWidth={2.6} />{/if}
                </span>
                <span class="candidate-title">
                  {game.title}
                  {#if game.releaseYear}<span class="year">({game.releaseYear})</span>{/if}
                </span>
              </button>
            {/each}
          </div>
        {/if}
      </section>
    </div>
  {/if}

  {#snippet footer()}
    <Button variant="danger" disabled={submitting} onclick={ignore}>Игнорировать</Button>
    <Button onclick={() => (open = false)}>Отмена</Button>
    <Button variant="primary" disabled={!selectedGameId || submitting} onclick={confirm}>Подтвердить</Button>
  {/snippet}
</Modal>

<style>
  .sections {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .block h4 {
    font-size: 1.3rem;
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
    font-size: 1.4rem;
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

  .muted {
    font-size: 1.4rem;
    color: var(--text-3);
  }

  .candidates {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    max-height: 18rem;
    overflow-y: auto;
  }

  .candidate {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0.9rem 1rem;
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    background: var(--surface);
    text-align: left;
    transition:
      background var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease);
  }

  .candidate:hover {
    border-color: var(--border-strong);
  }

  .candidate.selected {
    border-color: rgba(104, 117, 232, 0.55);
    background: var(--accent-subtle);
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
    border-color: var(--accent);
  }

  .candidate-title {
    flex: 1;
    min-width: 0;
    font-size: 1.4rem;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .year {
    color: var(--text-3);
    margin-left: 0.4rem;
  }

  .candidate-method {
    font-size: 1.2rem;
    color: var(--text-3);
    flex-shrink: 0;
  }

  .candidate-score {
    font-size: 1.3rem;
    font-weight: 600;
    color: var(--accent-text);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
    min-width: 4rem;
    text-align: right;
  }

  input {
    width: 100%;
    height: var(--control-md);
    padding: 0 1.2rem;
    margin-bottom: 0.8rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: 1.5rem;
    color: var(--text);
    outline: none;
    transition: border-color var(--dur) var(--ease);
  }

  input:focus {
    border-color: rgba(104, 117, 232, 0.55);
  }
</style>
