<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { ArrowLeft, Download, Gamepad2, House, Library, Menu, Monitor, Search, Settings, User, Users, WifiOff } from '@lucide/svelte';
  import { startBigPictureInput, type BigPictureCommand, type BigPictureDevice } from '../bigpicture/input';
  import { focusControl, moveFocus } from '../bigpicture/navigation';
  import { exitBigPicture, restoreBigPictureWindow } from '../bigpicture/mode';
  import { createBigPictureSession } from '../bigpicture/session';
  import { entry, popPage, pushPage, type Page, type Section } from '../bigpicture/router';
  import type { BigPicturePageHandle, RequestText, TextRequest } from '../bigpicture/contracts';
  import { launchGame } from '../services/library';
  import { runningGames } from '../stores/library';
  import { downloads } from '../stores/downloads';
  import { authState, isOffline, leaveGuest } from '../stores/user';
  import { selfUpdateStatus } from '../stores/selfupdate';
  import { t, locale } from '../i18n';
  import HomePage from '../bigpicture/components/HomePage.svelte';
  import LibraryPage from '../bigpicture/components/LibraryPage.svelte';
  import CatalogPage from '../bigpicture/components/CatalogPage.svelte';
  import GamePage from '../bigpicture/components/GamePage.svelte';
  import DownloadsPage from '../bigpicture/components/DownloadsPage.svelte';
  import SettingsPage from '../bigpicture/components/SettingsPage.svelte';
  import ProfilePage from '../bigpicture/components/ProfilePage.svelte';
  import SocialPage from '../bigpicture/components/SocialPage.svelte';
  import ScreenKeyboard from '../bigpicture/components/ScreenKeyboard.svelte';
  import '../bigpicture/styles.css';

  let shell: HTMLElement;
  let stage = $state<HTMLElement>();
  let pages = $state([entry({ name: 'home' })]);
  let menu = $state(false);
  let textRequest = $state<TextRequest | null>(null);
  let device = $state<BigPictureDevice>('keyboard');
  let exiting = $state(false);
  let failure = $state('');
  let disposed = false;
  let overlayFocus = '';
  let textFocus = '';
  let resolveText: ((value: string | null) => void) | undefined;
  let lastFocused: HTMLButtonElement | undefined;
  let lastFocusKey = '';
  const handles: Record<number, unknown> = {};
  const launchPlaces = new Map<string, { key: number; focus: string }>();
  const active = $derived(pages[pages.length - 1]);
  const applyingUpdate = $derived($selfUpdateStatus.state === 'applying');
  const currentDownloads = $derived($downloads.filter((item) => item.status !== 'completed'));
  function activeHandle() { return handles[active.key] as BigPicturePageHandle | undefined; }
  const sections = $derived.by(() => {
    $locale;
    return [
    { name: 'home' as const, title: $t('bp.home'), icon: House },
    { name: 'library' as const, title: $t('bp.library'), icon: Library },
    { name: 'catalog' as const, title: $t('bp.catalogTitle'), icon: Search },
    { name: 'downloads' as const, title: $t('bp.downloads'), icon: Download },
    { name: 'settings' as const, title: $t('bp.settingsTitle'), icon: Settings },
    { name: 'profile' as const, title: $t('bp.profileTitle'), icon: User },
    { name: 'social' as const, title: $t('bp.friendsTitle'), icon: Users },
    ];
  });

  function controls(root: HTMLElement | undefined): HTMLButtonElement[] {
    return root ? [...root.querySelectorAll<HTMLButtonElement>('button[data-bp-focus]:not(:disabled)')]
      .filter((node) => node.getClientRects().length > 0 && !node.closest('[inert], [hidden], [aria-hidden="true"]')) : [];
  }
  function scope(): HTMLElement | undefined {
    return [...shell.querySelectorAll<HTMLElement>('[data-bp-scope]')]
      .filter((node) => node.getClientRects().length > 0 && !node.closest('[hidden], [inert]')).at(-1) ?? stage;
  }
  function currentFocus(): string { return (document.activeElement as HTMLElement)?.dataset?.bpFocus ?? ''; }
  function focusKey(key: string, root = scope()): boolean {
    const target = controls(root).find((node) => node.dataset.bpFocus === key);
    if (!target) return false;
    focusControl(target); return true;
  }
  async function focusPage(key = active.focus) {
    await tick();
    if (disposed || applyingUpdate) return;
    const root = scope();
    if (key && focusKey(key, root)) return;
    const page = root === stage ? stage?.querySelector<HTMLElement>(`[data-bp-page="${active.key}"]`) : root;
    const buttons = controls(page ?? root);
    focusControl(buttons.find((node) => node.hasAttribute('data-bp-default')) ?? buttons[0] ?? controls(root)[0]);
  }
  async function navigate(page: Page) {
    pages = pushPage(pages, page, currentFocus()); failure = ''; await focusPage();
  }
  async function section(name: Section) {
    menu = false; pages = [entry({ name })]; failure = ''; await focusPage();
  }
  async function showMenu(show = true) {
    if (show) overlayFocus = currentFocus();
    menu = show; await tick();
    if (show) await focusPage('menu-resume');
    else if (!focusKey(overlayFocus)) await focusPage();
  }
  function back() {
    if (textRequest) { finishText(null); return; }
    if (menu) { void showMenu(false); return; }
    if (activeHandle()?.handleCommand?.('back')) return;
    if (pages.length > 1) { pages = popPage(pages); void focusPage(); }
    else if (active.page.name !== 'home') void section('home');
    else void showMenu();
  }
  const requestText: RequestText = (request) => {
    if (disposed || resolveText) return Promise.resolve(null);
    textFocus = currentFocus(); textRequest = request;
    return new Promise((resolve) => { resolveText = resolve; });
  };
  function finishText(value: string | null) {
    const resolve = resolveText; resolveText = undefined; textRequest = null;
    void tick().then(() => { if (!disposed) focusKey(textFocus); resolve?.(value); });
  }

  const session = createBigPictureSession({
    launch: launchGame,
    async onReturn(id) {
      if (disposed) return;
      const place = launchPlaces.get(id); launchPlaces.delete(id);
      const index = pages.findIndex((page) => page.key === place?.key);
      if (index >= 0) pages = pages.slice(0, index + 1);
      else pages = pushPage(pages, { name: 'game', id }, currentFocus());
      menu = false;
      await restoreBigPictureWindow();
      if (!disposed && !textRequest) await focusPage(place?.focus);
    },
    onReturnError() { failure = $t('bp.returnError'); },
  });
  async function play(id: string): Promise<boolean> {
    launchPlaces.set(id, { key: active.key, focus: currentFocus() });
    try { const launched = await session.launch(id); if (!launched) launchPlaces.delete(id); return launched; }
    catch (error) { launchPlaces.delete(id); throw error; }
  }
  async function leave(account = false) {
    if (exiting) return;
    exiting = true; failure = '';
    try {
      await exitBigPicture();
      if (account && $authState === 'guest') await leaveGuest();
    } catch { failure = $t('bp.exitError'); }
    finally { exiting = false; }
  }
  function command(command: BigPictureCommand) {
    if (!shell) return;
    if (command === 'back') { back(); return; }
    const root = scope();
    if (!root) return;
    if (!textRequest && !menu && activeHandle()?.handleCommand?.(command)) return;
    if (command === 'menu') {
      if (textRequest) finishText(null);
      else if (root === stage || menu) void showMenu(!menu);
      return;
    }
    if (command === 'previous' || command === 'next') {
      if (textRequest || menu || root !== stage) return;
      const index = Math.max(0, sections.slice(0, 4).findIndex((item) => item.name === active.page.name));
      void section(sections[(index + (command === 'next' ? 1 : -1) + 4) % 4].name); return;
    }
    if (command === 'confirm') {
      const element = document.activeElement;
      if (element instanceof HTMLButtonElement && controls(root).includes(element)) element.click();
      else void focusPage();
      return;
    }
    if (root === stage) {
      const page = stage.querySelector<HTMLElement>(`[data-bp-page="${active.key}"]`);
      if (page?.contains(document.activeElement)) {
        // The fixed header can appear geometrically closer than the previous
        // off-screen row. Walk the page first and reach the header at its top.
        if (moveFocus(page, command) || command !== 'up') return;
      }
    }
    moveFocus(root, command);
  }
  function trapTab(event: KeyboardEvent) {
    if (event.key !== 'Tab' || event.metaKey || event.ctrlKey || event.altKey || applyingUpdate) return;
    const nodes = controls(scope());
    if (!nodes.length) return;
    event.preventDefault();
    const index = nodes.indexOf(document.activeElement as HTMLButtonElement);
    focusControl(nodes[(index + (event.shiftKey ? -1 : 1) + nodes.length) % nodes.length]);
  }
  onMount(() => {
    const unsubscribe = runningGames.subscribe((ids) => session.observe(ids));
    const stopInput = startBigPictureInput({ onCommand: command, isEnabled: () => !applyingUpdate && !exiting, onDevice: (next) => device = next });
    const rememberFocus = (event: FocusEvent) => {
      if (event.target instanceof HTMLButtonElement && event.target.dataset.bpFocus) {
        lastFocused = event.target; lastFocusKey = event.target.dataset.bpFocus;
      }
    };
    shell.addEventListener('focusin', rememberFocus);
    void focusPage();
    // A completed request can remove/disable the focused button. Recover only
    // when focus is actually lost, never when live progress changes elsewhere.
    const observer = new MutationObserver(() => {
      if (disposed || applyingUpdate || !document.hasFocus() || textRequest) return;
      const buttons = controls(scope());
      if (buttons.includes(document.activeElement as HTMLButtonElement)) return;
      // Pending controls are often disabled for one RPC. Wait for that same
      // control to return instead of moving the user back to the page header.
      if (lastFocused?.isConnected && lastFocused.disabled && !lastFocused.closest('[hidden], [inert]')) return;
      if (!focusKey(lastFocusKey)) void focusPage();
    });
    observer.observe(shell, { childList: true, subtree: true, attributes: true, attributeFilter: ['disabled', 'hidden', 'inert'] });
    return () => {
      disposed = true; observer.disconnect(); shell.removeEventListener('focusin', rememberFocus); unsubscribe(); stopInput(); session.dispose();
      resolveText?.(null); resolveText = undefined;
      for (const key of Object.keys(handles)) delete handles[Number(key)];
    };
  });
</script>

<svelte:window onkeydown={trapTab} />
<div class="big-picture" data-big-picture bind:this={shell}>
  <div class="stage" bind:this={stage} inert={menu || !!textRequest || applyingUpdate}>
    <header>
      <div class="brand"><img src="/typhon.png" alt="Typhon" /><span>Big Picture</span></div>
      <nav aria-label={$t('bp.menu')}>
        {#each sections.slice(0, 4) as item}
          <button data-bp-focus={`nav-${item.name}`} data-bp-row="header" class:active={pages[0].page.name === item.name} onclick={() => section(item.name)} aria-label={item.title}><item.icon size="22" /><span>{item.title}</span>{#if item.name === 'downloads' && currentDownloads.length}<b>{currentDownloads.length}</b>{/if}</button>
        {/each}
      </nav>
      <div class="header-actions">
        {#if $isOffline}<span title={$t('bp.offline')}><WifiOff size="22" /></span>{/if}
        <button class="utility" data-bp-focus="menu" data-bp-row="header" onclick={() => showMenu()} aria-label={$t('bp.menu')}><Menu size="26" /></button>
      </div>
    </header>
    <main>
      {#each pages as page (page.key)}
        <div class="page-slot" data-bp-page={page.key} hidden={page.key !== active.key} inert={page.key !== active.key}>
          {#if page.page.name === 'home'}
            <HomePage bind:this={handles[page.key]} ongame={(id) => navigate({ name: 'game', id })} oncatalog={() => section('catalog')} onlaunch={play} />
          {:else if page.page.name === 'library'}
            <LibraryPage onback={back} ongame={(id) => navigate({ name: 'game', id })} oncatalog={() => section('catalog')} {requestText} />
          {:else if page.page.name === 'catalog'}
            <CatalogPage bind:this={handles[page.key]} onback={back} ongame={(id) => navigate({ name: 'game', id })} {requestText} />
          {:else if page.page.name === 'game'}
            <GamePage bind:this={handles[page.key]} id={page.page.id} onback={back} ondownloads={() => section('downloads')} onlaunch={play} {requestText} />
          {:else if page.page.name === 'downloads'}
            <DownloadsPage bind:this={handles[page.key]} onback={back} ongame={(id) => navigate({ name: 'game', id })} {requestText} />
          {:else if page.page.name === 'settings'}
            <SettingsPage bind:this={handles[page.key]} onback={back} {requestText} />
          {:else if page.page.name === 'profile'}
            <ProfilePage bind:this={handles[page.key]} onback={back} ongame={(id) => navigate({ name: 'game', id })} onaccount={() => leave(true)} {requestText} />
          {:else if page.page.name === 'social'}
            <SocialPage bind:this={handles[page.key]} onback={back} ongame={(id) => navigate({ name: 'game', id })} onaccount={() => leave(true)} {requestText} />
          {/if}
        </div>
      {/each}
    </main>
  </div>
  <footer>
    <div class="hints">
      <span><kbd>{device === 'keyboard' ? '↔ ↕' : '✥'}</kbd>{$t('bp.navigate')}</span>
      <span><kbd>{device === 'keyboard' ? 'Enter' : device === 'playstation' ? '✕' : device === 'xbox' ? 'A' : '1'}</kbd>{$t('bp.select')}</span>
      <span><kbd>{device === 'keyboard' ? 'Esc' : device === 'playstation' ? '○' : device === 'xbox' ? 'B' : '2'}</kbd>{$t('bp.back')}</span>
      <span class="extra-hint"><kbd>{device === 'keyboard' ? 'Q / E' : device === 'playstation' ? 'L1 / R1' : 'LB / RB'}</kbd>{$t('bp.shelves')}</span>
      <span class="extra-hint"><kbd>{device === 'keyboard' ? 'F10' : '☰'}</kbd>{$t('bp.menu')}</span>
    </div>
    <span class="input-device"><Gamepad2 size="20" />{device === 'keyboard' ? $t('bp.controllerHint') : $t('bp.controller')}</span>
  </footer>
  {#if menu}
    <div class="bp-veil">
      <div class="bp-dialog menu-dialog" role="dialog" aria-modal="true" aria-labelledby="bp-menu-title" tabindex="-1" data-bp-scope inert={!!textRequest || applyingUpdate}>
        <h2 id="bp-menu-title">{$t('bp.menu')}</h2>
        <div class="menu-items">
          <button class="bp-button" data-bp-focus="menu-resume" data-bp-default onclick={() => showMenu(false)}><ArrowLeft />{$t('bp.resume')}</button>
          {#each sections as item}<button class="bp-button" data-bp-focus={`menu-${item.name}`} onclick={() => section(item.name)}><item.icon />{item.title}</button>{/each}
          <button class="bp-button" data-bp-focus="menu-exit" disabled={exiting} onclick={() => leave()}><Monitor />{$t('bp.exit')}</button>
        </div>
        {#if failure}<p class="bp-error" role="alert">{failure}</p>{/if}
      </div>
    </div>
  {/if}
  {#if textRequest}<ScreenKeyboard request={textRequest} ondone={finishText} />{/if}
  {#if failure && !menu}<p class="bp-error floating" role="alert">{failure}</p>{/if}
</div>

<style>
  .big-picture { --bp-unit: clamp(16px, 1.25vw, 28px); position: fixed; inset: 0; z-index: 130; overflow: hidden; background: #090e16; color: #f3f5fa; font-size: var(--bp-unit); line-height: 1.45; }
  .stage { position: relative; height: calc(100% - 68px); display: flex; flex-direction: column; }
  header { padding: 18px 3vw 12px; display: flex; gap: 20px; justify-content: space-between; align-items: center; flex: none; border-bottom: 1px solid #ffffff0a; }
  .brand { display: flex; align-items: center; gap: 12px; font-size: 16px; white-space: nowrap; letter-spacing: .04em; }.brand img { width: 34px; height: 34px; }
  nav, .header-actions { display: flex; align-items: center; gap: 12px; }nav button { padding: 10px 14px; display: flex; align-items: center; gap: 9px; font-size: .83em; }.active { background: #ffffff18; }nav b { font-size: .75em; padding: 1px 6px; background: var(--accent); border-radius: 4px; }
  .utility { display: grid; place-items: center; min-width: 46px; min-height: 46px; }
  main { flex: 1; min-height: 0; position: relative; }.page-slot { height: 100%; overflow-y: auto; overflow-anchor: none; scrollbar-width: thin; scroll-padding: 22px; scroll-behavior: smooth; }.page-slot[hidden] { display: none; }
  footer { position: absolute; bottom: 0; inset-inline: 0; height: 68px; padding: 0 3vw; display: flex; justify-content: space-between; gap: 24px; align-items: center; background: #090e16f5; border-top: 1px solid #ffffff12; font-size: clamp(13px, .86vw, 20px); }
  .hints, .hints > span, .input-device { display: flex; align-items: center; gap: 9px; }.hints { gap: 20px; color: #bcc8d8; }kbd { display: inline-flex; justify-content: center; align-items: center; min-width: 26px; padding: 3px 6px; font: inherit; font-weight: 600; border: 1px solid #ffffff3b; border-radius: 6px; color: #fff; }.input-device { color: #8d9aaf; font-size: .9em; max-width: 230px; }
  .menu-dialog { width: min(760px, 95vw); }.menu-items { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-top: 20px; }.menu-items button { justify-content: flex-start; text-align: left; }.floating { position: absolute; bottom: 85px; left: 4vw; right: 4vw; }
  @media (max-width: 1150px) { .input-device { display: none; }.brand span { display: none; } }
  @media (max-width: 700px) { nav button span { display: none; }.extra-hint { display: none !important; }.menu-items { grid-template-columns: 1fr; } }
  @media (prefers-reduced-motion: reduce) { .page-slot { scroll-behavior: auto; } }
</style>
