<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte';
  import { Dices, Play, X } from '@lucide/svelte';
  import { t, errorCode, hasMessage } from '../../i18n';
  import { canPlay } from '../../library/launch';
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
  } from '../../game/randomPick';
  import type { LibraryGame } from '../../services/library';
  import { collections } from '../../stores/collections';
  import { libraryGenres } from '../../stores/genres';
  import { libraryGames } from '../../stores/library';
  import { gameArt, requestArt } from '../../stores/metadata';
  import { focusControl } from '../navigation';
  import Artwork from '../../components/Artwork.svelte';
  import { relativeDate } from '../../utils/format';

  let { onclose, onlaunch, ongame }: {
    onclose: () => void;
    onlaunch: (id: string) => Promise<boolean>;
    ongame: (id: string) => void;
  } = $props();

  type Phase = 'setup' | 'spinning' | 'result' | 'empty';

  let root: HTMLElement | undefined = $state();
  let filters = $state<PickFilters>(readFilters());
  let phase = $state<Phase>('setup');
  let shown = $state<LibraryGame | null>(null);
  let previousId = $state<string | null>(null);
  let launching = $state(false);
  let failure = $state('');
  let cancelSpin: (() => void) | undefined;

  const sources = $derived<PickSource[]>(['installed', 'all', 'backlog', ...($collections.length > 0 ? ['collection' as const] : [])]);
  const knownGenres = $derived(
    [...new Set($libraryGames.flatMap((game) => (game.canonicalGameId ? ($libraryGenres[game.canonicalGameId] ?? []) : [])))]
      .toSorted((a, b) => a.localeCompare(b)),
  );
  const sourceLabels = $derived<Record<PickSource, string>>({
    installed: $t('games.randomSourceInstalled'),
    all: $t('games.randomSourceAll'),
    backlog: $t('games.randomSourceBacklog'),
    collection: $t('games.randomSourceCollection'),
  });
  const collectionName = $derived($collections.find((item) => item.id === filters.collectionId)?.name ?? '');

  $effect(() => {
    if (!sources.includes(filters.source)) filters.source = 'installed';
  });
  $effect(() => {
    if (filters.genre && !knownGenres.includes(filters.genre)) filters.genre = '';
  });
  $effect(() => {
    if (filters.source === 'collection' && !$collections.some((item) => item.id === filters.collectionId)) {
      filters.collectionId = $collections[0]?.id ?? '';
    }
  });
  $effect(() => {
    writeFilters({ ...filters });
  });
  $effect(() => {
    const ids = $libraryGames.map((game) => game.canonicalGameId ?? '');
    untrack(() => requestArt(ids));
  });
  $effect(() => {
    phase;
    void tick().then(() => {
      const target = root?.querySelector<HTMLButtonElement>('[data-bp-default]:not(:disabled)');
      if (target) focusControl(target);
    });
  });

  function cycleSource() {
    filters.source = sources[(sources.indexOf(filters.source) + 1) % sources.length];
  }

  function cycleCollection() {
    const list = $collections;
    if (list.length === 0) return;
    const index = list.findIndex((item) => item.id === filters.collectionId);
    filters.collectionId = list[(index + 1) % list.length].id;
  }

  function cycleGenre() {
    const options = ['', ...knownGenres];
    filters.genre = options[(options.indexOf(filters.genre) + 1) % options.length];
  }

  function art(game: LibraryGame): string {
    return (game.canonicalGameId ? $gameArt[game.canonicalGameId]?.cover : '') || game.cover;
  }

  function spin() {
    cancelSpin?.();
    failure = '';
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
      ongame(game.id);
      return;
    }
    launching = true;
    failure = '';
    try {
      await onlaunch(game.id);
    } catch (error) {
      const code = errorCode(error);
      failure = hasMessage(code) ? $t(code) : $t('games.errorPlayFailed');
    } finally {
      launching = false;
    }
  }

  onDestroy(() => cancelSpin?.());
</script>

<div class="backdrop">
  <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="bp-random-title" tabindex="-1" bind:this={root} data-bp-scope>
    <header>
      <h2 id="bp-random-title">{$t('games.randomTitle')}</h2>
      <button class="bp-button" aria-label={$t('common.close')} data-bp-focus="random:close" onclick={onclose}><X /></button>
    </header>

    {#if phase === 'setup' || phase === 'empty'}
      <div class="filters">
        <button class="bp-button" data-bp-focus="random:source" onclick={cycleSource}>{$t('games.randomSourceLabel')}: {sourceLabels[filters.source]}</button>
        {#if filters.source === 'collection'}
          <button class="bp-button" data-bp-focus="random:collection" onclick={cycleCollection}>{collectionName}</button>
        {/if}
        <button class="bp-button" class:on={filters.stale} aria-pressed={filters.stale} data-bp-focus="random:stale" onclick={() => (filters.stale = !filters.stale)}>{$t('games.randomStale')}</button>
        {#if knownGenres.length > 0}
          <button class="bp-button" data-bp-focus="random:genre" onclick={cycleGenre}>{$t('games.randomGenreLabel')}: {filters.genre || $t('games.randomGenreAny')}</button>
        {/if}
      </div>
      {#if phase === 'empty'}<p class="empty" role="alert">{$t('games.randomEmpty')}</p>{/if}
      <div class="actions">
        <button class="bp-button bp-primary" data-bp-focus="random:spin" data-bp-default onclick={spin}><Dices />{$t('games.randomSpin')}</button>
      </div>
    {:else if shown}
      <div class="card" aria-live="polite">
        <div class="cover"><Artwork src={art(shown)} alt="" label={shown.title} ratio="3 / 4" radius="12px" /></div>
        <div class="info">
          <h3>{shown.title}</h3>
          <p>{phase === 'spinning' ? $t('games.randomSpinning') : shown.uninstalled ? $t('games.randomNotInstalled') : shown.lastPlayed ? `${$t('bp.lastPlayed')}: ${relativeDate(shown.lastPlayed)}` : $t('games.randomNeverPlayed')}</p>
        </div>
      </div>
      {#if failure}<p class="empty" role="alert">{failure}</p>{/if}
      {#if phase === 'result'}
        <div class="actions">
          <button class="bp-button bp-primary" data-bp-focus="random:play" data-bp-default disabled={launching} onclick={() => shown && play(shown)}>
            {#if canPlay(shown)}<Play />{$t('games.randomPlay')}{:else}{$t('games.randomOpen')}{/if}
          </button>
          <button class="bp-button" data-bp-focus="random:again" onclick={spin}><Dices />{$t('games.randomAgain')}</button>
          <button class="bp-button" data-bp-focus="random:filters" onclick={() => (phase = 'setup')}>{$t('games.randomFilters')}</button>
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .backdrop { position: fixed; z-index: 100; inset: 0; display: grid; place-items: center; padding: 2rem; background: rgba(0, 0, 0, .76); }
  .dialog { display: flex; width: min(72rem, 100%); max-height: min(90vh, 100rem); flex-direction: column; gap: 2rem; overflow: auto; padding: 2.4rem; border: 1px solid var(--border); border-radius: 1.8rem; background: var(--surface-2); box-shadow: 0 3rem 8rem rgba(0, 0, 0, .5); }
  header { display: flex; align-items: center; justify-content: space-between; gap: 2rem; }
  h2, h3, p { margin: 0; }
  .filters, .actions { display: flex; flex-wrap: wrap; gap: 1.2rem; }
  .filters .on { background: var(--accent); color: white; }
  .empty { color: var(--danger); }
  .card { display: flex; align-items: center; gap: 2.4rem; }
  .cover { flex: none; width: min(22rem, 34vw); }
  .info { display: flex; flex-direction: column; gap: 1rem; min-width: 0; }
  .info h3 { font-size: 1.6em; line-height: 1.3; overflow-wrap: anywhere; }
  .info p { color: var(--text-2); }
</style>
