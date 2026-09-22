<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { ArrowLeft, Clock, Download, Gamepad2, Menu, Monitor, Play, Square, Star, WifiOff, X } from '@lucide/svelte';
  import { buildShelves, resolveSelection, type ShelfID } from '../bigpicture/library';
  import { startBigPictureInput, type BigPictureCommand } from '../bigpicture/input';
  import { moveFocus } from '../bigpicture/navigation';
  import { exitBigPicture, restoreBigPictureWindow } from '../bigpicture/mode';
  import { createBigPictureSession } from '../bigpicture/session';
  import { launchGame, stopGame, type LibraryGame } from '../services/library';
  import { libraryGames, runningGames } from '../stores/library';
  import { gameArt, requestArt } from '../stores/metadata';
  import { downloads, statusLabels } from '../stores/downloads';
  import { isOffline } from '../stores/user';
  import { selfUpdateStatus } from '../stores/selfupdate';
  import { bytesSize, playtime, relativeDate } from '../utils/format';
  import { msg, errorCode, hasMessage } from '../i18n';
  import Artwork from './Artwork.svelte';

  type Panel = 'menu' | 'details' | 'downloads' | 'stop' | null;
  let stage: HTMLElement;
  let dialog = $state<HTMLElement>();
  let selectedId = $state('');
  let lastCard = '';
  let panel = $state<Panel>(null);
  let device = $state<'keyboard' | 'xbox' | 'playstation' | 'generic'>('keyboard');
  let busy = $state('');
  let exiting = $state(false);
  let failure = $state('');
  let disposed = false;
  let focusBeforePanel = '';
  let stopTarget = '';
  const launchFocus = new Map<string, string>();

  const shelves = $derived(buildShelves($libraryGames));
  const selected = $derived(resolveSelection(shelves, selectedId));
  const art = $derived(selected?.canonicalGameId ? $gameArt[selected.canonicalGameId] : undefined);
  const running = $derived(selected ? $runningGames.has(selected.id) : false);
  const currentDownloads = $derived($downloads.filter((item) => item.status !== 'completed'));
  const shelfTitles: Record<ShelfID, string> = {
    recent: msg('bp.recent'), favorites: msg('bp.favorites'), installed: msg('bp.installed'),
  };
  const applyingUpdate = $derived($selfUpdateStatus.state === 'applying');

  function errorText(error: unknown, fallback: string) {
    const code = errorCode(error);
    return hasMessage(code) ? msg(code) : fallback;
  }

  function focusables(root: HTMLElement | undefined): HTMLButtonElement[] {
    return root ? [...root.querySelectorAll<HTMLButtonElement>('[data-bp-focus]:not(:disabled)')]
      .filter((node) => node.getClientRects().length > 0) : [];
  }

  function focusKey(key: string, root = stage): boolean {
    const element = focusables(root).find((node) => node.dataset.bpFocus === key);
    if (!element) return false;
    element.focus({ preventScroll: true });
    element.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'instant' });
    return true;
  }

  async function focusCard(id = selected?.id) {
    await tick();
    if (disposed || panel) return;
    if (lastCard && focusKey(lastCard)) return;
    const card = focusables(stage).find((node) => node.dataset.gameId === id);
    if (card) {
      card.focus({ preventScroll: true });
      card.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'instant' });
    } else focusables(stage)[0]?.focus();
  }

  async function showPanel(next: Panel) {
    if (next === 'stop') stopTarget = selected?.id ?? '';
    if (!panel && next) focusBeforePanel = (document.activeElement as HTMLElement)?.dataset?.bpFocus ?? '';
    panel = next;
    failure = '';
    await tick();
    if (disposed) return;
    if (next) {
      (dialog?.querySelector<HTMLButtonElement>('[data-bp-default]') ?? focusables(dialog)[0])?.focus();
    } else if (!focusKey(focusBeforePanel)) await focusCard();
  }

  function select(game: LibraryGame, key: string) {
    if (selectedId !== game.id) failure = '';
    selectedId = game.id;
    lastCard = key;
  }

  const session = createBigPictureSession({
    launch: launchGame,
    async onReturn(id) {
      if (disposed) return;
      selectedId = id;
      lastCard = launchFocus.get(id) ?? '';
      launchFocus.delete(id);
      panel = null;
      await restoreBigPictureWindow();
      if (!disposed) await focusCard(id);
    },
    onReturnError() { failure = msg('bp.returnError'); },
  });

  async function play() {
    if (!selected || busy || running) return;
    const id = selected.id;
    busy = 'launch';
    failure = '';
    launchFocus.set(id, lastCard);
    try {
      if (!await session.launch(id)) launchFocus.delete(id);
    } catch (error) {
      launchFocus.delete(id);
      failure = errorText(error, msg('games.errorPlayFailed'));
    } finally {
      busy = '';
      await tick();
      if (!disposed && document.hasFocus()) {
        if (panel) focusables(dialog)[0]?.focus();
        else await focusCard();
      }
    }
  }

  async function stop() {
    if (!stopTarget || busy) return;
    const id = stopTarget;
    if (!$runningGames.has(id)) { await showPanel(null); return; }
    busy = 'stop';
    failure = '';
    try {
      await stopGame(id);
      if (!disposed && panel === 'stop') await showPanel('details');
    } catch (error) {
      failure = errorText(error, msg('bp.stopError'));
    } finally {
      busy = '';
      await tick();
      if (!disposed && panel) focusables(dialog)[0]?.focus();
    }
  }

  async function leave() {
    if (exiting) return;
    exiting = true;
    failure = '';
    try { await exitBigPicture(); }
    catch { failure = msg('bp.exitError'); }
    finally { exiting = false; }
  }

  function back() {
    if (panel === 'stop') void showPanel('details');
    else if (panel === 'downloads') void showPanel('menu');
    else void showPanel(panel ? null : 'menu');
  }

  function changeShelf(direction: number) {
    if (panel || shelves.length === 0) return;
    const id = lastCard.split(':')[0];
    const index = shelves.findIndex((shelf) => shelf.id === id);
    const next = shelves[(Math.max(0, index) + direction + shelves.length) % shelves.length];
    focusKey(`${next.id}:${next.games[0].id}`);
  }

  function command(command: BigPictureCommand) {
    const root = panel ? dialog : stage;
    if (!root) return;
    if (command === 'back') { back(); return; }
    if (command === 'menu') { void showPanel(panel === 'menu' ? null : 'menu'); return; }
    if (command === 'previous' || command === 'next') { changeShelf(command === 'next' ? 1 : -1); return; }
    if (panel === 'downloads' && (command === 'up' || command === 'down')) {
      dialog?.querySelector('.download-list')?.scrollBy({ top: command === 'down' ? 220 : -220 });
      return;
    }
    if (command === 'confirm') {
      const element = document.activeElement;
      if (element instanceof HTMLButtonElement && root.contains(element) && !element.disabled) element.click();
      else focusables(root)[0]?.focus();
      return;
    }
    moveFocus(root, command);
  }

  function trapTab(event: KeyboardEvent) {
    if (event.key !== 'Tab' || event.metaKey || event.ctrlKey || event.altKey || applyingUpdate) return;
    const nodes = focusables(panel ? dialog : stage);
    if (nodes.length === 0) return;
    event.preventDefault();
    const index = nodes.indexOf(document.activeElement as HTMLButtonElement);
    nodes[(index + (event.shiftKey ? -1 : 1) + nodes.length) % nodes.length]?.focus();
  }

  $effect(() => {
    const ids = shelves.flatMap((shelf) => shelf.games.map((game) => game.canonicalGameId ?? ''));
    untrack(() => requestArt(ids));
  });

  $effect(() => {
    const id = selected?.id ?? '';
    untrack(() => {
      if (selectedId !== id) {
        selectedId = id;
        lastCard = '';
        if (panel === 'details' || panel === 'stop') void showPanel(null);
        if (stage) void focusCard();
      }
    });
  });

  onMount(() => {
    const unsubscribe = runningGames.subscribe((ids) => session.observe(ids));
    const stopInput = startBigPictureInput({
      onCommand: command,
      isEnabled: () => !applyingUpdate && !exiting,
      onDevice: (next) => { device = next; },
    });
    void focusCard();
    return () => { disposed = true; unsubscribe(); stopInput(); session.dispose(); };
  });
</script>

<svelte:window onkeydown={trapTab} />

<div class="big-picture" data-big-picture>
  <div class="backdrop" aria-hidden="true">
    {#if art?.hero || art?.cover || selected?.cover}
      <Artwork src={art?.hero || art?.cover || selected?.cover || ''} />
    {/if}
  </div>
  <div class="stage" bind:this={stage} inert={panel !== null || applyingUpdate}>
    <header>
      <div class="brand"><img src="/typhon.png" alt="Typhon" /><span>Big Picture</span></div>
      <div class="header-actions">
        {#if $isOffline}<span class="offline"><WifiOff size="20" />{msg('bp.offline')}</span>{/if}
        <button data-bp-focus="downloads" class="utility" onclick={() => showPanel('downloads')} aria-label={msg('bp.downloads')}>
          <Download size="24" />{#if currentDownloads.length}<span>{currentDownloads.length}</span>{/if}
        </button>
        <button data-bp-focus="menu" class="utility" onclick={() => showPanel('menu')} aria-label={msg('bp.menu')}><Menu size="26" /></button>
      </div>
    </header>
    <main>
      {#if selected}
        <section class="hero" aria-label={selected.title}>
          <div class="eyebrow"><span class="dot"></span>{running ? msg('bp.running') : msg('bp.library')}</div>
          <h1>{selected.title}</h1>
          <div class="hero-meta">
            {#if selected.playtimeSeconds > 0}<span><Clock size="20" />{playtime(selected.playtimeSeconds)}</span>{/if}
            {#if selected.favorite}<span><Star size="20" />{msg('bp.favorites')}</span>{/if}
            {#if selected.version}<span>{selected.version}</span>{/if}
          </div>
          <div class="hero-actions">
            <button data-bp-focus="play" class="primary" disabled={!!busy} onclick={() => running ? showPanel('stop') : play()}>
              {#if running}<Square size="23" fill="currentColor" />{:else}<Play size="24" fill="currentColor" />{/if}
              {busy === 'launch' ? msg('bp.starting') : running ? msg('ui.stop') : msg('ui.play')}
            </button>
            <button data-bp-focus="details" class="secondary" onclick={() => showPanel('details')}>{msg('bp.details')}</button>
          </div>
        </section>
        <div class="shelves">
          {#each shelves as shelf (shelf.id)}
            <section class="shelf" aria-label={shelfTitles[shelf.id]}>
              <h2>{shelfTitles[shelf.id]}<span>{shelf.games.length}</span></h2>
              <div class="rail">
                {#each shelf.games as game (game.id)}
                  {@const key = `${shelf.id}:${game.id}`}
                  <button class="game" data-bp-focus={key} data-game-id={game.id} class:selected={selected?.id === game.id}
                    aria-label={game.title} onfocus={() => select(game, key)} onclick={() => { select(game, key); void showPanel('details'); }}>
                    <div class="cover"><Artwork src={(game.canonicalGameId ? $gameArt[game.canonicalGameId]?.cover : '') || game.cover} alt="" label={game.title} ratio="3 / 4" radius="12px" /></div>
                    <span class="game-title">{game.title}</span>
                    {#if $runningGames.has(game.id)}<span class="running-badge"><span class="dot"></span>{msg('bp.running')}</span>{/if}
                  </button>
                {/each}
              </div>
            </section>
          {/each}
        </div>
      {:else}
        <section class="empty">
          <Gamepad2 size="76" strokeWidth={1.2} />
          <h1>{msg('bp.emptyTitle')}</h1><p>{msg('bp.emptyText')}</p>
          <button data-bp-focus="exit-empty" class="primary" disabled={exiting} onclick={leave}><Monitor size="24" />{msg('bp.exit')}</button>
        </section>
      {/if}
    </main>
  </div>

  {#if failure && !panel}<div class="failure floating" role="alert">{failure}</div>{/if}

  <footer>
    <div class="hints">
      <span><kbd>{device === 'keyboard' ? '↔ ↕' : '✥'}</kbd>{msg('bp.navigate')}</span>
      <span><kbd>{device === 'keyboard' ? 'Enter' : device === 'playstation' ? '✕' : device === 'xbox' ? 'A' : '1'}</kbd>{msg('bp.select')}</span>
      <span><kbd>{device === 'keyboard' ? 'Esc' : device === 'playstation' ? '○' : device === 'xbox' ? 'B' : '2'}</kbd>{msg('bp.back')}</span>
      <span class="extra-hint"><kbd>{device === 'keyboard' ? 'Q / E' : device === 'playstation' ? 'L1 / R1' : 'LB / RB'}</kbd>{msg('bp.shelves')}</span>
      <span class="extra-hint"><kbd>{device === 'keyboard' ? 'F10' : '☰'}</kbd>{msg('bp.menu')}</span>
    </div>
    <span class="input-device"><Gamepad2 size="20" /><span>{device === 'keyboard' ? msg('bp.controllerHint') : msg('bp.controller')}</span></span>
  </footer>

  {#if panel}
    <div class="veil">
      <div class="panel" class:wide={panel === 'downloads'} role="dialog" aria-modal="true" aria-labelledby="bp-panel-title" tabindex="-1" bind:this={dialog}>
        {#if panel === 'menu'}
          <span class="eyebrow">TYPHON</span><h2 id="bp-panel-title">{msg('bp.menu')}</h2>
          <div class="menu-items">
            <button data-bp-focus="resume" data-bp-default class="secondary" onclick={() => showPanel(null)}><ArrowLeft size="24" />{msg('bp.resume')}</button>
            <button data-bp-focus="menu-downloads" class="secondary" onclick={() => showPanel('downloads')}><Download size="24" />{msg('bp.downloads')}<span class="count">{currentDownloads.length}</span></button>
            <button data-bp-focus="exit" class="secondary" disabled={exiting} onclick={leave}><Monitor size="24" />{msg('bp.exit')}</button>
          </div>
        {:else if panel === 'downloads'}
          <h2 id="bp-panel-title">{msg('bp.downloads')}</h2>
          <p class="muted">{msg('bp.downloadsHint')}</p>
          <div class="download-list">
            {#each currentDownloads as item (item.id)}
              <div class="download-item">
                <h3>{item.name}</h3><div class="download-meta"><span>{statusLabels(item.status)}</span><span>{Math.round(Math.max(0, Math.min(1, item.progress || 0)) * 100)}%</span></div>
                <progress max="1" value={Math.max(0, Math.min(1, item.progress || 0))} aria-label={item.name}></progress>
                <span class="muted">{bytesSize(item.downloaded)} / {bytesSize(item.total)}</span>
              </div>
            {:else}<p class="empty-downloads">{msg('bp.noDownloads')}</p>{/each}
          </div>
          <button data-bp-focus="download-back" data-bp-default class="secondary" onclick={back}><ArrowLeft size="24" />{msg('bp.back')}</button>
        {:else if panel === 'stop' && selected}
          <h2 id="bp-panel-title">{msg('bp.stopTitle')}</h2><p>{selected.title}</p><p class="muted">{msg('bp.stopText')}</p>
          <div class="panel-actions">
            <button data-bp-focus="cancel-stop" data-bp-default class="secondary" onclick={back}>{msg('common.cancel')}</button>
            <button data-bp-focus="confirm-stop" class="primary danger" disabled={!!busy} onclick={stop}><Square size="22" />{busy === 'stop' ? msg('bp.stopping') : msg('ui.stop')}</button>
          </div>
        {:else if selected}
          <span class="eyebrow">{running ? msg('bp.running') : msg('bp.installed')}</span>
          <h2 id="bp-panel-title">{selected.title}</h2>
          <dl>
            <div><dt>{msg('bp.playtime')}</dt><dd>{playtime(selected.playtimeSeconds)}</dd></div>
            {#if selected.lastPlayed}<div><dt>{msg('bp.lastPlayed')}</dt><dd>{relativeDate(selected.lastPlayed)}</dd></div>{/if}
          </dl>
          <div class="panel-actions">
            <button data-bp-focus="detail-play" data-bp-default class="primary" disabled={!!busy} onclick={() => running ? showPanel('stop') : play()}>
              {#if running}<Square size="23" />{:else}<Play size="24" fill="currentColor" />{/if}
              {busy === 'launch' ? msg('bp.starting') : running ? msg('ui.stop') : msg('ui.play')}
            </button>
            <button data-bp-focus="detail-back" class="secondary" onclick={back}><X size="24" />{msg('bp.back')}</button>
          </div>
        {/if}
        {#if failure}<div class="failure" role="alert">{failure}</div>{/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .big-picture { --bp-unit: clamp(16px, 1.25vw, 28px); position: fixed; inset: 0; z-index: 130; overflow: hidden; background: #090e16; color: #f3f5fa; font-size: var(--bp-unit); line-height: 1.45; }
  .backdrop { position: absolute; inset: 0; opacity: .42; pointer-events: none; }
  .backdrop::after { content: ''; position: absolute; inset: 0; background: linear-gradient(90deg, #090e16 0%, #090e1638 85%), linear-gradient(0deg, #090e16 10%, #090e1666 70%); }
  .stage { position: relative; height: calc(100% - 78px); display: flex; flex-direction: column; }
  header { padding: 24px 4vw 12px; display: flex; justify-content: space-between; align-items: center; flex: none; }
  .brand { display: flex; align-items: center; gap: 16px; font-size: 18px; letter-spacing: .06em; font-weight: 550; }
  .brand img { width: 37px; height: 37px; }
  .header-actions, .hero-meta, .hero-meta > span, .hero-actions, .offline { display: flex; align-items: center; gap: 20px; }
  .offline { gap: 9px; font-size: .85em; color: #c0c9d6; }
  button { border-radius: 12px; transition: background 120ms, outline-color 120ms, transform 120ms; outline: 3px solid transparent; outline-offset: 5px; }
  button:focus { outline-color: #f5f7ff; box-shadow: 0 0 0 7px var(--accent); }
  button:hover { background-color: #ffffff1a; }
  button:disabled { opacity: .6; cursor: wait; }
  .utility { min-height: 48px; min-width: 48px; display: flex; justify-content: center; align-items: center; gap: 8px; padding: 10px; }
  main { flex: 1; min-height: 0; overflow-y: auto; scrollbar-width: thin; scroll-padding: 20px; }
  .hero { padding: clamp(20px, 4vh, 60px) 4vw 24px; min-height: 32vh; display: flex; flex-direction: column; align-items: flex-start; justify-content: flex-end; gap: 16px; }
  .eyebrow { display: flex; align-items: center; gap: 10px; color: #c7cfdf; text-transform: uppercase; font-size: .72em; letter-spacing: .12em; font-weight: 600; }
  .dot { flex: none; width: 8px; height: 8px; background: var(--accent); border-radius: 50%; }
  h1 { max-width: 78%; font-size: clamp(32px, 4vw, 88px); letter-spacing: -.035em; line-height: 1.08; text-wrap: balance; overflow-wrap: anywhere; }
  .hero-meta { min-height: 26px; font-size: .85em; color: #c7cfdf; }
  .hero-meta > span { gap: 8px; }
  .primary, .secondary { display: inline-flex; align-items: center; justify-content: center; gap: 13px; min-height: 54px; padding: 13px 24px; font-size: 1em; font-weight: 550; }
  .primary { background: var(--accent); color: white; min-width: 172px; }
  .primary:hover { background: var(--accent-hover); }
  .secondary { background: #ffffff13; border: 1px solid #ffffff1c; }
  .danger { background: #ad3949; }
  .shelves { padding: 0 0 24px; }
  .shelf { margin-top: 22px; }
  .shelf h2 { display: flex; gap: 14px; align-items: center; padding: 0 4vw; font-size: 1.1em; }
  .shelf h2 > span { font-size: .7em; color: #8d9aaf; font-weight: 450; }
  .rail { display: flex; gap: clamp(18px, 1.7vw, 34px); overflow-x: auto; padding: 17px 4vw 18px; scroll-padding-inline: 4vw; scrollbar-width: none; }
  .rail::-webkit-scrollbar { display: none; }
  .game { flex: 0 0 clamp(138px, 13vw, 290px); min-width: 0; text-align: left; align-self: flex-start; padding: 0; background: #111a27; position: relative; overflow: visible; }
  .game:focus { transform: translateY(-3px); background: #243049; }
  .cover { overflow: hidden; border-radius: 12px 12px 0 0; }
  .game-title { display: block; padding: 12px 13px; font-size: .78em; font-weight: 550; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .running-badge { position: absolute; bottom: 48px; left: 8px; right: 8px; display: flex; align-items: center; gap: 7px; background: #0c132deb; border-radius: 5px; padding: 5px 8px; font-size: .6em; }
  .empty { min-height: 100%; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; gap: 24px; padding: 40px; }
  .empty h1 { font-size: 2.3em; max-width: 900px; }.empty p { color: #b2bfd1; max-width: 680px; }
  footer { position: absolute; bottom: 0; inset-inline: 0; height: 78px; padding: 0 4vw; display: flex; justify-content: space-between; gap: 24px; align-items: center; background: #090e16f5; border-top: 1px solid #ffffff12; font-size: clamp(13px, .86vw, 20px); }
  .hints, .hints > span, .input-device { display: flex; align-items: center; gap: 10px; }
  .hints { gap: 22px; color: #bcc8d8; } kbd { display: inline-flex; justify-content: center; align-items: center; min-width: 26px; padding: 3px 6px; font: inherit; font-weight: 600; border: 1px solid #ffffff3b; border-radius: 6px; color: #fff; }
  .input-device { color: #8d9aaf; font-size: .9em; max-width: 230px; }
  .veil { position: absolute; inset: 0 0 78px; display: flex; align-items: center; justify-content: center; padding: 28px; background: #030711b8; backdrop-filter: blur(18px); }
  .panel { width: min(660px, 90vw); max-height: 100%; overflow-y: auto; padding: clamp(24px, 3vw, 55px); background: #141e2d; border: 1px solid #ffffff20; border-radius: 24px; box-shadow: 0 24px 100px #0008; display: flex; flex-direction: column; gap: 24px; }
  .panel.wide { width: min(840px, 90vw); }.panel h2 { font-size: 1.8em; overflow-wrap: anywhere; }
  .menu-items { display: flex; flex-direction: column; gap: 18px; }.menu-items button { justify-content: flex-start; }.count { margin-left: auto; }
  .panel-actions { display: flex; gap: 18px; flex-wrap: wrap; }.muted, dt { color: #a9b7cb; font-size: .9em; }
  dl { display: grid; gap: 14px; }dl > div { display: flex; gap: 20px; justify-content: space-between; }dd { text-align: right; }
  .failure { color: #ffd5d7; background: #642a3a; padding: 16px 20px; border: 1px solid #e38b9a70; border-radius: 10px; font-size: .85em; }
  .floating { position: absolute; bottom: 100px; left: 4vw; right: 4vw; }
  .download-list { display: grid; gap: 24px; max-height: 42vh; overflow-y: auto; }.download-item h3 { font-size: 1em; }.download-meta { display: flex; justify-content: space-between; font-size: .85em; color: #a9b7cb; margin-top: 8px; }
  progress { width: 100%; height: 8px; appearance: none; border: none; border-radius: 8px; overflow: hidden; background: #ffffff1a; }progress::-webkit-progress-bar { background: #ffffff1a; }progress::-webkit-progress-value { background: var(--accent); }progress::-moz-progress-bar { background: var(--accent); }
  .empty-downloads { padding: 25px 0; color: #b8c4d6; }
  @media (max-width: 1000px) { .input-device { display: none; } h1 { max-width: 90%; } }
  @media (max-height: 760px) { header { padding-top: 14px; }.hero { gap: 10px; padding-top: 16px; min-height: 27vh; } .hero-meta { min-height: 20px; }.game { flex-basis: clamp(122px, 12vw, 200px); } footer { height: 62px; }.stage { height: calc(100% - 62px); }.veil { bottom: 62px; } }
  @media (max-width: 650px) { .extra-hint { display: none !important; }.brand { font-size: 14px; } header { padding-inline: 20px; }.hero { padding-inline: 24px; }h1 { font-size: 30px; }.hero-actions { gap: 12px; }.primary, .secondary { min-width: 0; padding: 12px 18px; }.panel { padding: 24px; } }
  @media (prefers-reduced-motion: reduce) { button { transition: none; }.game:focus { transform: none; } }
</style>
