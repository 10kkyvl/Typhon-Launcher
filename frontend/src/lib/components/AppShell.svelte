<script lang="ts">
  import type { Snippet } from 'svelte';
  import { route } from '../stores/router';
  import { authState, isOffline } from '../stores/user';
  import { needsSocialConsent } from '../stores/social';
  import { chatOpen } from '../stores/messaging';
  import { themeLayout } from '../stores/layout';
  import { scrollmemory } from '../utils/scrollmemory';
  import ActivityDock from './ActivityDock.svelte';
  import ChatPanel from './ChatPanel.svelte';
  import NetworkBanner from './NetworkBanner.svelte';
  import OfflineBanner from './OfflineBanner.svelte';
  import ShellBackdrop from './ShellBackdrop.svelte';
  import Sidebar from './Sidebar.svelte';
  import GameQuickMenu from './GameQuickMenu.svelte';
  import Toasts from './Toasts.svelte';
  import Topbar from './Topbar.svelte';

  let { children }: { children?: Snippet } = $props();

  const chatShown = $derived($authState === 'authenticated' && !$needsSocialConsent);

  // Toasts stand on top of the activity dock, as one column in the corner.
  let dockHeight = $state(0);
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
     a toast raised from an open dialog is seen and its action can be pressed. -->
<div
  class="corner toast-region"
  class:above-chat={chatShown && !$chatOpen}
  class:beside-chat={chatShown && $chatOpen}
  class:sidebar-right={$themeLayout.side === 'right'}
  style:--dock-offset={dockHeight > 0 ? `calc(${dockHeight}px + 0.8rem)` : '0px'}
  data-layer-keep
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
    bottom: calc(var(--corner-bottom) + var(--dock-offset, 0px));
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
