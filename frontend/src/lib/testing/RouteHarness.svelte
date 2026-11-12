<script lang="ts">
  import AppShell from '../components/AppShell.svelte';
  import PageHeader from '../components/PageHeader.svelte';
  import { goBack, navigate, route } from '../stores/router';

  // The main window's shell with three pages, as App.svelte routes them: the library with a game to
  // open, the game page whose heading comes once its details have loaded (`loaded`), and the friends
  // page. For AppShell.focus.dom.test.ts.
  let { loaded = true }: { loaded?: boolean } = $props();
</script>

<AppShell>
  {#if $route.name === 'library'}
    <PageHeader title="Library" />
    <button type="button" onclick={() => navigate('game', { id: 'celeste' })}>Open Celeste</button>
  {:else if $route.name === 'game'}
    {#if loaded}
      <h1 tabindex="-1">Celeste</h1>
    {:else}
      <p>Loading</p>
    {/if}
    <button type="button" onclick={goBack}>Back to the library</button>
  {:else if $route.name === 'friends'}
    <PageHeader title="Friends" />
  {/if}
</AppShell>
