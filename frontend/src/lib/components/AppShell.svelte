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
  import Sidebar from './Sidebar.svelte';
  import GameQuickMenu from './GameQuickMenu.svelte';
  import Toasts from './Toasts.svelte';
  import Topbar from './Topbar.svelte';

  let { children }: { children?: Snippet } = $props();

  const chatShown = $derived($authState === 'authenticated' && !$needsSocialConsent);
</script>

<div class="shell" class:right={$themeLayout.side === 'right'} data-part="app-shell">
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
>
  <Toasts />
  {#if $route.name !== 'downloads'}
    <ActivityDock />
  {/if}
</div>

<GameQuickMenu />
{#if chatShown}
  <ChatPanel />
{/if}

<style>
  .corner {
    position: fixed;
    z-index: 120;
    right: 2.4rem;
    bottom: 2.4rem;
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.8rem;
    pointer-events: none;
  }

  .corner.above-chat {
    bottom: calc(2.4rem + 4.8rem + 1.2rem);
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
    height: 100vh;
    min-width: 100rem;
    overflow: hidden;
  }

  .shell.right {
    flex-direction: row-reverse;
  }

  :global(:root) {
    --sidebar-now: var(--sidebar-w);
  }

  @media (max-width: 1140px) {
    :global(:root) {
      --sidebar-now: 6.4rem;
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
    padding: 0 var(--page-x) var(--space-10);
  }

  .page {
    max-width: var(--page-max);
    margin: 0 auto;
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
