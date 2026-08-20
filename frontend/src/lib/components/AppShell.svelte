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
        {@render children?.()}
      </main>
    {/key}
  </div>
</div>

<Toasts />

<style>
  .shell {
    display: flex;
    height: 100vh;
    min-width: 1000px;
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
    padding: var(--space-2) var(--page-x) var(--space-10);
    animation: page-in var(--dur) var(--ease);
  }

  @keyframes page-in {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
