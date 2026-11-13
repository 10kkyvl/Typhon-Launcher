<script lang="ts">
  import { onDestroy, tick } from 'svelte';
  import { Dices, Play } from '@lucide/svelte';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import EmptyState from './EmptyState.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
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
  const uid = $props.id();

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

  // The footer the spin was asked from stays while it spins, its button busy, so the focus on it stays.
  let spunFrom = $state<'setup' | 'result'>('setup');
  let spinSlot = $state<HTMLElement>();
  let againSlot = $state<HTMLElement>();

  function spin(event: MouseEvent) {
    // Whether the pressed spin button held the focus: only then is there a focus to hand on.
    const held = event.currentTarget instanceof HTMLElement && document.activeElement === event.currentTarget;
    cancelSpin?.();
    spunFrom = phase === 'result' ? 'result' : 'setup';
    const candidates = pickCandidates($libraryGames, filters, $libraryGenres, $collections, Date.now());
    const final = pickRandom(candidates, previousId);
    if (!final) {
      shown = null;
      phase = 'empty';
      if (held) void keepFocus();
      return;
    }
    previousId = final.id;
    if (reducedMotion()) {
      shown = final;
      phase = 'result';
      if (held) void keepFocus();
      return;
    }
    phase = 'spinning';
    cancelSpin = runSpin(
      spinFrames(candidates, final),
      (game) => (shown = game),
      (game) => {
        shown = game;
        phase = 'result';
        if (held) void keepFocus();
      },
    );
  }

  // When the footer changed under the spin button that held the focus, that focus goes to the spin
  // button of the new footer: "Again" after a result, so a second Enter spins again rather than starting a game.
  async function keepFocus() {
    await tick();
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected) return;
    (phase === 'result' ? againSlot : spinSlot)?.querySelector<HTMLElement>('button')?.focus();
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

<!-- dismissible while spinning: the spin is only an animation; closing ends it and loses nothing. -->
<Modal dismissible={!launching} open title={msg('games.randomTitle')} width="52rem" {onclose}>
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
            <Select options={collectionOptions} bind:value={filters.collectionId} width="100%" label={msg('games.randomSourceCollection')} />
          {:else}
            <EmptyState size="inline" title={msg('games.randomNoCollections')} />
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
          <span class="label" id="{uid}-genre-label">{msg('games.randomGenreLabel')}</span>
          <Select options={genreOptions} bind:value={filters.genre} width="100%" labelledby="{uid}-genre-label" />
        </div>
      {/if}
      {#if phase === 'empty'}
        <Notice tone="warning" compact>{msg('games.randomEmpty')}</Notice>
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
    {#if phase === 'setup' || phase === 'empty' || (phase === 'spinning' && spunFrom === 'setup')}
      <Button onclick={onclose}>{msg('common.close')}</Button>
      <span class="slot" bind:this={spinSlot}>
        <Button variant="primary" busy={phase === 'spinning'} onclick={spin}>
          <Dices size="1.6rem" strokeWidth={1.8} />
          {phase === 'spinning' ? msg('games.randomSpinning') : msg('games.randomSpin')}
        </Button>
      </span>
    {:else if shown}
      <Button disabled={phase === 'spinning'} onclick={() => (phase = 'setup')}>{msg('games.randomFilters')}</Button>
      <span class="slot" bind:this={againSlot}>
        <Button busy={phase === 'spinning'} onclick={spin}>
          <Dices size="1.6rem" strokeWidth={1.8} />
          {phase === 'spinning' ? msg('games.randomSpinning') : msg('games.randomAgain')}
        </Button>
      </span>
      <Button variant="primary" disabled={phase === 'spinning'} busy={launching} onclick={() => play(shown!)}>
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

  /* Holds a footer button for the focus to find, without a box of its own. */
  .slot {
    display: contents;
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
