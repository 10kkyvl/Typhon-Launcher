<script lang="ts">
  import { onDestroy } from 'svelte';
  import { Dices, Play } from '@lucide/svelte';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import Modal from './Modal.svelte';
  import Select from './Select.svelte';
  import { msg } from '../i18n';
  import { libraryErrorText } from '../library/errors';
  import { canPlay } from '../library/launch';
  import {
    pickCandidates,
    pickRandom,
    readFilters,
    reducedMotion,
    runSpin,
    spinFrames,
    writeFilters,
    type PickFilters,
    type PickSource,
  } from '../game/randomPick';
  import { playGame, type LibraryGame } from '../services/library';
  import { collections } from '../stores/collections';
  import { libraryGenres } from '../stores/genres';
  import { libraryGames } from '../stores/library';
  import { gameArt } from '../stores/metadata';
  import { navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { relativeDate } from '../utils/format';

  let { onclose }: { onclose: () => void } = $props();

  type Phase = 'setup' | 'spinning' | 'result' | 'empty';

  let filters = $state<PickFilters>(readFilters());
  let phase = $state<Phase>('setup');
  let shown = $state<LibraryGame | null>(null);
  let previousId = $state<string | null>(null);
  let launching = $state(false);
  let cancelSpin: (() => void) | undefined;

  const sources: { id: PickSource; label: () => string }[] = [
    { id: 'installed', label: () => msg('games.randomSourceInstalled') },
    { id: 'all', label: () => msg('games.randomSourceAll') },
    { id: 'backlog', label: () => msg('games.randomSourceBacklog') },
    { id: 'collection', label: () => msg('games.randomSourceCollection') },
  ];

  const knownGenres = $derived(
    [
      ...new Set(
        $libraryGames.flatMap((game) => (game.canonicalGameId ? ($libraryGenres[game.canonicalGameId] ?? []) : [])),
      ),
    ].toSorted((a, b) => a.localeCompare(b)),
  );

  const genreOptions = $derived([
    { id: '', label: msg('games.randomGenreAny') },
    ...knownGenres.map((genre) => ({ id: genre, label: genre })),
  ]);

  const collectionOptions = $derived($collections.map((item) => ({ id: item.id, label: item.name })));

  $effect(() => {
    if (filters.genre && !knownGenres.includes(filters.genre)) filters.genre = '';
  });

  $effect(() => {
    if (filters.source !== 'collection') return;
    if ($collections.some((item) => item.id === filters.collectionId)) return;
    filters.collectionId = $collections[0]?.id ?? '';
  });

  $effect(() => {
    writeFilters({ ...filters });
  });

  function art(game: LibraryGame): string {
    return (game.canonicalGameId ? $gameArt[game.canonicalGameId]?.cover : '') || game.cover;
  }

  function spin() {
    cancelSpin?.();
    const candidates = pickCandidates($libraryGames, filters, $libraryGenres, $collections, Date.now());
    const final = pickRandom(candidates, previousId);
    if (!final) {
      shown = null;
      phase = 'empty';
      return;
    }
    previousId = final.id;
    if (reducedMotion()) {
      shown = final;
      phase = 'result';
      return;
    }
    phase = 'spinning';
    cancelSpin = runSpin(
      spinFrames(candidates, final),
      (game) => (shown = game),
      (game) => {
        shown = game;
        phase = 'result';
      },
    );
  }

  async function play(game: LibraryGame) {
    if (launching) return;
    if (!canPlay(game)) {
      navigate('game', { id: game.id });
      onclose();
      return;
    }
    launching = true;
    try {
      await playGame(game.id);
      onclose();
    } catch (err) {
      toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
    } finally {
      launching = false;
    }
  }

  onDestroy(() => cancelSpin?.());
</script>

<Modal open title={msg('games.randomTitle')} width="52rem" {onclose}>
  {#if phase === 'setup' || phase === 'empty'}
    <div class="filters">
      <div class="group">
        <span class="label">{msg('games.randomSourceLabel')}</span>
        <div class="chips">
          {#each sources as source (source.id)}
            <Chip selected={filters.source === source.id} onclick={() => (filters.source = source.id)}>{source.label()}</Chip>
          {/each}
        </div>
        {#if filters.source === 'collection'}
          {#if collectionOptions.length > 0}
            <Select options={collectionOptions} bind:value={filters.collectionId} width="100%" />
          {:else}
            <p class="hint">{msg('games.randomNoCollections')}</p>
          {/if}
        {/if}
      </div>
      <div class="group">
        <div class="chips">
          <Chip selected={filters.stale} onclick={() => (filters.stale = !filters.stale)}>{msg('games.randomStale')}</Chip>
        </div>
      </div>
      {#if knownGenres.length > 0}
        <div class="group">
          <span class="label">{msg('games.randomGenreLabel')}</span>
          <Select options={genreOptions} bind:value={filters.genre} width="100%" />
        </div>
      {/if}
      {#if phase === 'empty'}
        <p class="empty" role="alert">{msg('games.randomEmpty')}</p>
      {/if}
    </div>
  {:else if shown}
    <div class="card" class:spinning={phase === 'spinning'} aria-live="polite">
      <div class="cover">
        <Artwork src={art(shown)} alt="" label={shown.title} ratio="3 / 4" radius="var(--radius-md)" />
      </div>
      <div class="info">
        <h3>{shown.title}</h3>
        {#if phase === 'result'}
          <p class="meta">
            {shown.uninstalled
              ? msg('games.randomNotInstalled')
              : shown.lastPlayed
                ? relativeDate(shown.lastPlayed)
                : msg('games.randomNeverPlayed')}
          </p>
        {:else}
          <p class="meta">{msg('games.randomSpinning')}</p>
        {/if}
      </div>
    </div>
  {/if}

  {#snippet footer()}
    {#if phase === 'setup' || phase === 'empty'}
      <Button onclick={onclose}>{msg('common.close')}</Button>
      <Button variant="primary" onclick={spin}>
        <Dices size="1.6rem" strokeWidth={1.8} />
        {msg('games.randomSpin')}
      </Button>
    {:else if phase === 'spinning'}
      <Button disabled>{msg('games.randomSpinning')}</Button>
    {:else if shown}
      <Button onclick={() => (phase = 'setup')}>{msg('games.randomFilters')}</Button>
      <Button onclick={spin}>
        <Dices size="1.6rem" strokeWidth={1.8} />
        {msg('games.randomAgain')}
      </Button>
      <Button variant="primary" disabled={launching} onclick={() => play(shown!)}>
        {#if canPlay(shown)}
          <Play size="1.6rem" strokeWidth={1.8} />
          {msg('games.randomPlay')}
        {:else}
          {msg('games.randomOpen')}
        {/if}
      </Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .filters {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .label {
    color: var(--text-2);
    font-size: var(--font-sm);
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .hint,
  .empty {
    margin: 0;
    color: var(--text-2);
    line-height: 1.55;
  }

  .empty {
    color: var(--warning);
  }

  .card {
    display: flex;
    align-items: center;
    gap: var(--space-6);
  }

  .cover {
    flex: none;
    width: 18rem;
  }

  .info h3 {
    margin: 0 0 var(--space-3);
    font-size: var(--font-xl);
    line-height: 1.3;
  }

  .meta {
    margin: 0;
    color: var(--text-2);
  }

  .card.spinning .info h3 {
    opacity: 0.7;
  }
</style>
