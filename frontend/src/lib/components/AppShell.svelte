<script lang="ts">
  import type { Snippet } from 'svelte';
  import { route } from '../stores/router';
  import { authState, isOffline } from '../stores/user';
  import { needsSocialConsent } from '../stores/social';
  import { chatOpen } from '../stores/messaging';
  import { themeLayout } from '../stores/layout';
  import { topModal } from '../layers';
  import { scrollmemory } from '../utils/scrollmemory';
  import ActivityDock from './ActivityDock.svelte';
  import ChatPanel from './ChatPanel.svelte';
  import NetworkBanner from './NetworkBanner.svelte';
  import OfflineBanner from './OfflineBanner.svelte';
  import ShellBackdrop from './ShellBackdrop.svelte';
  import Sidebar from './Sidebar.svelte';
  import GameQuickMenu from './GameQuickMenu.svelte';
  import { TOAST_GAP_REM, TOAST_MIN_WIDTH_REM, toastSpot, type ToastEnd } from './toastSpot';
  import Toasts from './Toasts.svelte';
  import Topbar from './Topbar.svelte';

  let { children }: { children?: Snippet } = $props();

  const chatShown = $derived($authState === 'authenticated' && !$needsSocialConsent);

  // Toasts stand on top of the activity dock, as one column in the corner.
  let dockHeight = $state(0);

  // Under a dialog the stack never lies over the dialog's panel: it keeps its corner, moves to the
  // top, narrows into the gutter beside the panel, or asks the dialog on top to end above it
  // (toastSpot.ts). What is asked of the dialog is `--toast-room` on its layer root, which Modal
  // reads; a layer without a panel (the lightbox) is asked nothing.
  let toastRegion = $state<HTMLElement>();
  let toastsAt = $state<ToastEnd>('bottom');
  const ROOM = '--toast-room';

  $effect(() => {
    const region = toastRegion;
    const root = $topModal;
    const panel = root?.querySelector('[data-part="dialog"]');
    const stack = region?.firstElementChild;
    if (!region || !stack || !root || !panel) {
      toastsAt = 'bottom';
      return;
    }

    // `fresh`: the panel is measured as it rests, so room it is making is taken back first. That lets
    // a scrolled dialog body jump, so it is done only when the dialog on top or the window changed;
    // otherwise a dialog that makes room keeps making it until the last toast is gone.
    const place = (fresh: boolean) => {
      // The stack is measured as wide as it likes to be.
      region.style.maxWidth = '';
      if (fresh) root.style.removeProperty(ROOM);
      const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
      const spot = toastSpot(panel.getBoundingClientRect(), region.getBoundingClientRect(), stack.getBoundingClientRect(), {
        minWidth: TOAST_MIN_WIDTH_REM * rem,
        gap: TOAST_GAP_REM * rem,
        floor: root.getBoundingClientRect().bottom,
        reserved: root.style.getPropertyValue(ROOM) !== '',
      });
      toastsAt = spot.at;
      if (spot.width !== null) region.style.maxWidth = `${spot.width}px`;
      if (spot.room > 0) root.style.setProperty(ROOM, `${spot.room}px`);
      else root.style.removeProperty(ROOM);
    };

    // Placing changes sizes that are observed (the stack narrows, the panel shortens). Done inside the
    // observer's own callback that would be a resize loop, so a change of size is answered in the
    // next frame, once for all that changed.
    let frame: number | undefined;
    let wholly = false;
    const later = (fresh: boolean) => {
      wholly ||= fresh;
      frame ??= requestAnimationFrame(() => {
        frame = undefined;
        const all = wholly;
        wholly = false;
        place(all);
      });
    };
    // The document element stands for the window: no listener on `window` is needed.
    const viewport = new ResizeObserver(() => later(true));
    viewport.observe(document.documentElement);
    const sizes = new ResizeObserver(() => later(false));
    for (const element of [region, stack, panel]) sizes.observe(element);
    // The panel enters smaller and lower than it rests: once its own animation is over (not one of
    // something inside it) it is measured afresh.
    const settled = (event: Event) => {
      if (event.target === panel) place(true);
    };
    panel.addEventListener('animationend', settled);
    place(true);

    return () => {
      if (frame !== undefined) cancelAnimationFrame(frame);
      viewport.disconnect();
      sizes.disconnect();
      panel.removeEventListener('animationend', settled);
      region.style.maxWidth = '';
      root.style.removeProperty(ROOM);
    };
  });
</script>

<div class="shell" class:right={$themeLayout.side === 'right'} data-part="app-shell">
  <ShellBackdrop />
  <Sidebar />
  <div class="main" data-part="main">
    <Topbar />
    {#if $isOffline}
      <OfflineBanner />
    {/if}
    <NetworkBanner />
    {#key $route}
      <main class="content" use:scrollmemory>
        <div class="page" data-part="page">
          {@render children?.()}
        </div>
      </main>
    {/key}
  </div>
</div>

<div
  class="corner"
  class:above-chat={chatShown && !$chatOpen}
  class:beside-chat={chatShown && $chatOpen}
  class:sidebar-right={$themeLayout.side === 'right'}
  bind:clientHeight={dockHeight}
>
  {#if $route.name !== 'downloads'}
    <ActivityDock />
  {/if}
</div>

<!-- Its own region above dialogs, left live while they make the app inert (data-layer-keep):
     a toast raised from an open dialog is seen and its action can be pressed. The region is the whole
     column the stack can stand in, from under the title bar down to the corner, and lets presses
     through; the stack stands at its bottom, or at its top while a dialog's panel takes the bottom,
     and is narrowed while it stands beside the panel (toastSpot.ts). -->
<div
  class="corner toast-region"
  class:at-top={toastsAt === 'top'}
  class:above-chat={chatShown && !$chatOpen}
  class:beside-chat={chatShown && $chatOpen}
  class:sidebar-right={$themeLayout.side === 'right'}
  style:--dock-offset={dockHeight > 0 ? `calc(${dockHeight}px + 0.8rem)` : '0px'}
  data-layer-keep
  bind:this={toastRegion}
>
  <Toasts />
</div>

<GameQuickMenu />
{#if chatShown}
  <ChatPanel />
{/if}

<style>
  .corner {
    --corner-bottom: 2.4rem;
    position: fixed;
    z-index: var(--z-corner);
    right: 2.4rem;
    bottom: var(--corner-bottom);
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.8rem;
    pointer-events: none;
  }

  .corner.above-chat {
    --corner-bottom: calc(2.4rem + 4.8rem + 1.2rem);
  }

  .toast-region {
    z-index: var(--z-toast);
    top: calc(var(--titlebar-h) + var(--space-2));
    bottom: calc(var(--corner-bottom) + var(--dock-offset, 0px));
    justify-content: flex-end;
  }

  .toast-region.at-top {
    justify-content: flex-start;
  }

  .corner.beside-chat {
    right: calc(2.4rem + 38rem + 1.2rem);
  }

  .corner.sidebar-right {
    right: calc(2.4rem + var(--sidebar-now));
  }

  .corner.sidebar-right.beside-chat {
    right: calc(2.4rem + var(--sidebar-now) + 38rem + 1.2rem);
  }

  .corner > :global(*) {
    pointer-events: auto;
  }

  .shell {
    display: flex;
    height: calc(100vh - var(--titlebar-h));
    min-width: 100rem;
    overflow: hidden;
  }

  .shell.right {
    flex-direction: row-reverse;
  }

  /* What the sidebar island takes from the window edge: its width and the gap outside it. */
  :global(:root) {
    --sidebar-now: calc(var(--sidebar-w) + var(--island-gap));
  }

  @media (max-width: 1140px) {
    :global(:root) {
      --sidebar-now: calc(var(--sidebar-rail-w) + var(--island-gap));
    }
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .content {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
  }

  /* The inset and width are the route's Page container's; this wrapper only animates the page in. */
  .page {
    animation: page-in var(--dur-panel) var(--ease);
  }

  @media (min-width: 2200px) {
    .content {
      --page-x: var(--space-12);
    }
  }

  @keyframes page-in {
    from {
      opacity: 0;
      transform: translateY(0.6rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
