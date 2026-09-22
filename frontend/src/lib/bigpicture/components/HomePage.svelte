<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { Clock, Gamepad2, Play, Square, Star } from '@lucide/svelte';
  import { buildShelves, resolveSelection, type ShelfID } from '../library';
  import type { BigPictureCommand } from '../input';
  import { focusControl } from '../navigation';
  import { libraryGames, runningGames } from '../../stores/library';
  import { gameArt, requestArt } from '../../stores/metadata';
  import { playtime } from '../../utils/format';
  import { t, errorCode, hasMessage } from '../../i18n';
  import Artwork from '../../components/Artwork.svelte';
  let { ongame, oncatalog, onlaunch }: { ongame: (id: string) => void; oncatalog: () => void; onlaunch: (id: string) => Promise<boolean> } = $props();
  let root: HTMLElement;
  let selectedId = $state('');
  let lastCard = '';
  let busy = $state(false);
  let failure = $state('');
  const shelves = $derived(buildShelves($libraryGames));
  const selected = $derived(resolveSelection(shelves, selectedId));
  const art = $derived(selected?.canonicalGameId ? $gameArt[selected.canonicalGameId] : undefined);
  const running = $derived(selected ? $runningGames.has(selected.id) : false);
  const titles: Record<ShelfID, string> = $derived({ recent: $t('bp.recent'), favorites: $t('bp.favorites'), installed: $t('bp.installed') });

  async function play() {
    if (!selected || busy) return;
    if (running) { ongame(selected.id); return; }
    busy = true; failure = '';
    try { await onlaunch(selected.id); }
    catch (error) { const code = errorCode(error); failure = hasMessage(code) ? $t(code) : $t('games.errorPlayFailed'); }
    finally { busy = false; await tick(); focusControl(root.querySelector<HTMLButtonElement>('[data-bp-focus="home-play"]') ?? undefined); }
  }
  export function handleCommand(command: BigPictureCommand): boolean {
    if (command !== 'next' && command !== 'previous') return false;
    const index = shelves.findIndex((shelf) => shelf.id === lastCard.split(':')[0]);
    const next = shelves[(Math.max(index, 0) + (command === 'next' ? 1 : -1) + shelves.length) % shelves.length];
    if (next) focusControl([...root.querySelectorAll<HTMLButtonElement>('[data-game-id]')].find((node) => node.dataset.bpFocus === `${next.id}:${next.games[0].id}`));
    return true;
  }
  $effect(() => { const ids = shelves.flatMap((shelf) => shelf.games.map((game) => game.canonicalGameId ?? '')); untrack(() => requestArt(ids)); });
</script>

<div class="home" bind:this={root}>
  <div class="backdrop" aria-hidden="true">{#if art?.hero || art?.cover || selected?.cover}<Artwork src={art?.hero || art?.cover || selected?.cover || ''} />{/if}</div>
  {#if selected}
    <section class="hero" aria-label={selected.title}>
      <div class="eyebrow"><span class="dot"></span>{running ? $t('bp.running') : $t('bp.library')}</div>
      <h1>{selected.title}</h1>
      <div class="hero-meta">
        {#if selected.playtimeSeconds > 0}<span><Clock size="20" />{playtime(selected.playtimeSeconds)}</span>{/if}
        {#if selected.favorite}<span><Star size="20" />{$t('bp.favorites')}</span>{/if}
        {#if selected.version}<span>{selected.version}</span>{/if}
      </div>
      <div class="bp-actions">
        <button data-bp-focus="home-play" data-bp-row="hero" class="bp-button bp-primary" disabled={busy} onclick={play}>
          {#if running}<Square size="23" />{:else}<Play size="24" fill="currentColor" />{/if}{busy ? $t('bp.starting') : running ? $t('ui.stop') : $t('ui.play')}
        </button>
        <button data-bp-focus="home-details" data-bp-row="hero" class="bp-button" onclick={() => ongame(selected.id)}>{$t('bp.details')}</button>
      </div>
      {#if failure}<p class="bp-error" role="alert">{failure}</p>{/if}
    </section>
    <div class="shelves">
      {#each shelves as shelf (shelf.id)}
        <section class="shelf" aria-label={titles[shelf.id]}>
          <h2>{titles[shelf.id]}<span>{shelf.games.length}</span></h2>
          <div class="rail">
            {#each shelf.games as game (game.id)}
              {@const key = `${shelf.id}:${game.id}`}
              <button class="game" data-bp-focus={key} data-bp-row={shelf.id} data-game-id={game.id} data-bp-default={shelf === shelves[0] && game === shelf.games[0] ? '' : undefined}
                aria-label={game.title} onfocus={() => { selectedId = game.id; lastCard = key; failure = ''; }} onclick={() => ongame(game.id)}>
                <div class="cover"><Artwork src={(game.canonicalGameId ? $gameArt[game.canonicalGameId]?.cover : '') || game.cover} alt="" label={game.title} ratio="3 / 4" radius="12px" /></div>
                <span class="game-title">{game.title}</span>
                {#if $runningGames.has(game.id)}<span class="running-badge"><span class="dot"></span>{$t('bp.running')}</span>{/if}
              </button>
            {/each}
          </div>
        </section>
      {/each}
    </div>
  {:else}
    <section class="bp-empty"><Gamepad2 size="76" strokeWidth={1.2} /><h1>{$t('bp.emptyTitle')}</h1><p>{$t('bp.emptyText')}</p><button data-bp-focus="home-catalog" data-bp-row="empty" data-bp-default class="bp-button bp-primary" onclick={oncatalog}>{$t('bp.openCatalog')}</button></section>
  {/if}
</div>

<style>
  .home { position: relative; min-height: 100%; }.backdrop { position: absolute; inset: 0 0 auto; height: 75vh; opacity: .45; pointer-events: none; }.backdrop::after { content: ''; position: absolute; inset: 0; background: linear-gradient(90deg, #090e16 0%, #090e1638 85%), linear-gradient(0deg, #090e16 10%, #090e1666 70%); }
  .hero, .shelves { position: relative; }.hero { padding: clamp(20px, 4vh, 60px) 4vw 24px; min-height: 32vh; display: flex; flex-direction: column; align-items: flex-start; justify-content: flex-end; gap: 16px; }
  .eyebrow { display: flex; align-items: center; gap: 10px; color: #c7cfdf; text-transform: uppercase; font-size: .72em; letter-spacing: .12em; font-weight: 600; }.dot { flex: none; width: 8px; height: 8px; background: var(--accent); border-radius: 50%; }
  h1 { max-width: 78%; font-size: clamp(32px, 4vw, 88px); letter-spacing: -.035em; line-height: 1.08; overflow-wrap: anywhere; }.hero h1 { height: 2.16em; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .hero-meta { min-height: 26px; font-size: .85em; color: #c7cfdf; display: flex; align-items: center; gap: 20px; }.hero-meta > span { display: flex; align-items: center; gap: 8px; }
  .shelves { padding-bottom: 24px; }.shelf { margin-top: 22px; }.shelf h2 { display: flex; gap: 14px; align-items: center; padding: 0 4vw; font-size: 1.1em; }.shelf h2 > span { font-size: .7em; color: #8d9aaf; font-weight: 450; }
  .rail { display: flex; gap: clamp(18px, 1.7vw, 34px); overflow-x: auto; padding: 17px 4vw 18px; scroll-padding-inline: 4vw; scrollbar-width: none; scroll-behavior: smooth; }.rail::-webkit-scrollbar { display: none; }
  .game { flex: 0 0 clamp(138px, 13vw, 290px); min-width: 0; text-align: left; align-self: flex-start; padding: 0; background: #111a27; position: relative; overflow: visible; }.game:focus { background: #243049; }.cover { overflow: hidden; border-radius: 12px 12px 0 0; }.game-title { display: block; padding: 12px 13px; font-size: .78em; font-weight: 550; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .running-badge { position: absolute; bottom: 48px; left: 8px; right: 8px; display: flex; align-items: center; gap: 7px; background: #0c132deb; border-radius: 5px; padding: 5px 8px; font-size: .6em; }
  @media (max-height: 760px) { .hero { gap: 10px; padding-top: 16px; min-height: 27vh; }.hero-meta { min-height: 20px; }.game { flex-basis: clamp(122px, 12vw, 200px); } }
  @media (prefers-reduced-motion: reduce) { .rail { scroll-behavior: auto; } }
</style>
