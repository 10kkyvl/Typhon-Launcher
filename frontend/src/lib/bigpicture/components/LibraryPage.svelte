<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { ArrowLeft, Search, Star, SlidersHorizontal } from '@lucide/svelte';
  import { t } from '../../i18n';
  import { libraryGames, runningGames } from '../../stores/library';
  import { gameArt, requestArt } from '../../stores/metadata';
  import { filterLibrary, type LibraryFilter, type LibrarySort } from '../library';
  import type { RequestText } from '../contracts';
  import { focusControl } from '../navigation';
  import Artwork from '../../components/Artwork.svelte';
  let { onback, ongame, oncatalog, requestText }: { onback: () => void; ongame: (id: string) => void; oncatalog: () => void; requestText: RequestText } = $props();
  let query = $state('');
  let filter = $state<LibraryFilter>('all');
  let sort = $state<LibrarySort>('title');
  let root: HTMLElement;
  const filters: LibraryFilter[] = ['all', 'installed', 'favorites', 'uninstalled'];
  const sorts: LibrarySort[] = ['title', 'recent', 'playtime', 'added'];
  const games = $derived(filterLibrary($libraryGames, query, filter, sort));
  const filterLabels = $derived({ all: $t('bp.allGames'), installed: $t('bp.installed'), favorites: $t('bp.favorites'), uninstalled: $t('bp.notInstalled') });
  const sortLabels = $derived({ title: $t('bp.sortTitle'), recent: $t('bp.lastPlayed'), playtime: $t('bp.playtime'), added: $t('bp.sortAdded') });
  async function search() {
    const next = await requestText({ title: $t('bp.librarySearch'), initialValue: query, maxLength: 150 });
    if (next !== null) query = next;
    await tick();
    focusControl(root.querySelector<HTMLButtonElement>('[data-bp-focus="library-search"]') ?? undefined);
  }
  $effect(() => { const ids = games.map((game) => game.canonicalGameId ?? ''); untrack(() => requestArt(ids)); });
</script>

<section class="bp-page" bind:this={root}>
  <div class="bp-page-header"><button class="bp-button" data-bp-focus="library-back" onclick={onback} aria-label={$t('bp.back')}><ArrowLeft /></button><h1>{$t('bp.library')} <span>{games.length}</span></h1></div>
  <div class="bp-actions filters">
    <button class="bp-button" data-bp-focus="library-search" data-bp-default onclick={search}><Search />{query || $t('bp.librarySearch')}</button>
    <button class="bp-button" data-bp-focus="library-filter" onclick={() => filter = filters[(filters.indexOf(filter) + 1) % filters.length]}><SlidersHorizontal />{filterLabels[filter]}</button>
    <button class="bp-button" data-bp-focus="library-sort" onclick={() => sort = sorts[(sorts.indexOf(sort) + 1) % sorts.length]}>{$t('bp.sort')}: {sortLabels[sort]}</button>
    {#if query || filter !== 'all'}<button class="bp-button" data-bp-focus="library-reset" onclick={() => { query = ''; filter = 'all'; }}>{$t('bp.reset')}</button>{/if}
  </div>
  <div class="library-grid">
    {#each games as game (game.id)}
      <button class="library-card" data-bp-focus={`library-game:${game.id}`} onclick={() => ongame(game.id)}>
        <Artwork src={(game.canonicalGameId ? $gameArt[game.canonicalGameId]?.cover : '') || game.cover} alt="" label={game.title} ratio="3 / 4" radius="12px" />
        <strong>{game.title}</strong>
        <span>{#if game.favorite}<Star size="16" fill="currentColor" />{/if}{game.uninstalled ? $t('bp.notInstalled') : $runningGames.has(game.id) ? $t('bp.running') : $t('bp.installed')}</span>
      </button>
    {:else}
      <div class="bp-empty"><h2>{$t('bp.noMatches')}</h2><p>{$t('bp.libraryEmptyHint')}</p><button class="bp-button bp-primary" data-bp-focus="library-catalog" onclick={oncatalog}>{$t('bp.openCatalog')}</button></div>
    {/each}
  </div>
</section>

<style>
  h1 span { color: #94a5bc; font-size: .55em; }.filters { margin-bottom: 30px; }.filters button { max-width: 100%; overflow-wrap: anywhere; }
  .library-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(clamp(140px, 13vw, 230px), 1fr)); gap: 32px 24px; }
  .library-card { display: flex; flex-direction: column; text-align: left; gap: 10px; min-width: 0; border-radius: 12px; }
  .library-card :global(.artwork) { width: 100%; }.library-card strong { font-size: .95em; line-height: 1.35; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .library-card span { color: #afbdd0; font-size: .7em; display: flex; align-items: center; gap: 7px; }.bp-empty { grid-column: 1 / -1; }
</style>
