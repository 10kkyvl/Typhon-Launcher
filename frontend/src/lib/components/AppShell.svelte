<script lang="ts">
  import type { Snippet } from 'svelte';
  import { route } from '../stores/router';
  import Sidebar from './Sidebar.svelte';
  import Toasts from './Toasts.svelte';
  import Topbar from './Topbar.svelte';

  let { children }: { children?: Snippet } = $props();
</script>

<div class="shell">
  <Sidebar />
  <div class="main">
    <Topbar />
    {#key $route}
      <main class="content">
        <div class="page">
          {@render children?.()}
        </div>
      </main>
    {/key}
  </div>
</div>

<Toasts />

<style>
  .shell {
    display: flex;
    height: 100vh;
    min-width: 100rem;
    overflow: hidden;
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
      padding-left: var(--space-12);
      padding-right: var(--space-12);
    }
  }

  @keyframes page-in {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }
</style>
