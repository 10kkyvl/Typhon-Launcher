<script lang="ts">
  import AppShell from '../components/AppShell.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import PageHeader from '../components/PageHeader.svelte';
  import { goBack, navigate, route } from '../stores/router';

  // The main window's shell with three pages, as App.svelte routes them: the library with a game to
  // open, the game page whose heading comes once its details have loaded (`loaded`) or that says the
  // game is not there (`missing`, as GameDetails does), and the friends page. A new `generation`
  // builds the shell again, as App.svelte does when the language is switched. For
  // AppShell.focus.dom.test.ts.
  let { loaded = true, missing = false, generation = 0 }: { loaded?: boolean; missing?: boolean; generation?: number } = $props();
</script>

{#key generation}
<AppShell>
  {#if $route.name === 'library'}
    <PageHeader title="Library" />
    <button type="button" onclick={() => navigate('game', { id: 'celeste' })}>Open Celeste</button>
  {:else if $route.name === 'game'}
    {#if missing}
      <EmptyState level={1} title="Game not found" />
    {:else if loaded}
      <h1 tabindex="-1">Celeste</h1>
    {:else}
      <p>Loading</p>
    {/if}
    <button type="button" onclick={goBack}>Back to the library</button>
  {:else if $route.name === 'friends'}
    <PageHeader title="Friends" />
  {/if}
</AppShell>
{/key}
